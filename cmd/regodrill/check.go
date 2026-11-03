package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/open-policy-agent/opa/v1/loader"
	"github.com/open-policy-agent/opa/v1/rego"
)

// `regodrill check` parses and compiles Rego files, like `opa check`, so that one binary serves both the
// editor's checks of embedded Rego (the Rego Syntax Support extension) and mutation testing.
//
// It follows `opa check` of the OPA version regodrill is built with (cmd/check.go, checkModules, without
// --bundle, --schema, --capabilities, --strict and --rego-v1): only .rego files are loaded, they are compiled
// together with the capabilities of this OPA version, and the errors go to stderr with exit code 1, as text
// or, with --format json, as OPA's presentation.Output: {"errors": [{"message", "code", "location",
// "details"}]}, indented by two spaces (internal/presentation, NewOutputErrors and JSON). Nothing is printed
// on success. cli_test.go compares the outputs with an opa binary when one is on the PATH.

// exitCheckFailed is the exit code of `opa check` when a file does not parse or compile.
const exitCheckFailed = 1

func checkCommand(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "pretty", "output format: pretty or json")
	v0 := flags.Bool("v0-compatible", false, "parse Rego v0 (the default is Rego v1)")
	maxErrors := flags.Int("max-errors", ast.CompileErrorLimitDefault,
		"number of errors to allow before compilation stops early (0 or less: no limit)")
	flags.IntVar(maxErrors, "m", ast.CompileErrorLimitDefault, "shorthand for --max-errors")
	var ignore listFlag
	flags.Var(&ignore, "ignore", "skip files and directories whose name matches this pattern, like opa check (repeatable)")
	flags.Usage = func() {
		fmt.Fprint(stderr, "Usage: regodrill check [flags] <path> [path...]\n\nParse and compile Rego files, like opa check.\n\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *format != "pretty" && *format != "json" {
		fmt.Fprintf(stderr, "error: unknown format %q: use pretty or json\n", *format)
		return exitUsage
	}
	if flags.NArg() == 0 {
		fmt.Fprintln(stderr, "error: specify at least one file")
		return exitUsage
	}
	version := ast.RegoV1
	if *v0 {
		version = ast.RegoV0
	}
	err := checkModules(flags.Args(), version, *maxErrors, ignore)
	if err == nil {
		return exitOK
	}
	if *format == "json" {
		enc := json.NewEncoder(stderr)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(checkOutput{Errors: checkErrors(err)}); encErr != nil {
			fmt.Fprintln(stderr, encErr.Error())
		}
	} else {
		fmt.Fprintln(stderr, err)
	}
	return exitCheckFailed
}

// checkModules loads and compiles the .rego files under paths, as `opa check` does without --bundle.
func checkModules(paths []string, version ast.RegoVersion, errLimit int, ignore []string) error {
	capabilities := ast.CapabilitiesForThisVersion(ast.CapabilitiesRegoVersion(version))
	l := loader.NewFileLoader().
		WithRegoVersion(version).
		WithProcessAnnotation(true).
		WithBundleLazyLoadingMode(bundle.HasExtension()).
		WithCapabilities(capabilities)
	result, err := l.Filtered(paths, func(abspath string, info fs.FileInfo, depth int) bool {
		// opa check loads only .rego files (cmd/filters.go, ignoredOnlyRego).
		if !info.IsDir() && filepath.Ext(info.Name()) != bundle.RegoExt {
			return true
		}
		for _, pattern := range ignore {
			if loader.GlobExcludeName(pattern, 1)(abspath, info, depth) {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	compiler := ast.NewCompiler().
		SetErrorLimit(errLimit).
		WithCapabilities(capabilities).
		WithEnablePrintStatements(true).
		WithUseTypeCheckAnnotations(true)
	if compiler.Compile(result.ParsedModules()); compiler.Failed() {
		return compiler.Errors
	}
	return nil
}

// checkOutput and checkError have the JSON form of OPA's presentation.Output and presentation.OutputError.
type checkOutput struct {
	Errors []checkError `json:"errors,omitempty"`
}

type checkError struct {
	Message  string        `json:"message"`
	Code     string        `json:"code,omitempty"`
	Location *ast.Location `json:"location,omitempty"`
	Details  any           `json:"details,omitempty"`
}

// checkErrors flattens err like presentation.NewOutputErrors: structured OPA errors keep their code and
// location, lists of errors are expanded, and other errors keep only their message.
func checkErrors(err error) []checkError {
	var out []checkError
	switch e := err.(type) {
	case *ast.Error:
		out = []checkError{{Code: e.Code, Message: e.Message, Location: e.Location}}
		if e.Details != nil {
			out[0].Details = e.Details
		}
	case ast.Errors:
		for _, inner := range e {
			if inner != nil {
				out = append(out, checkErrors(inner)...)
			}
		}
	case loader.Errors:
		for _, inner := range e {
			if inner != nil {
				out = append(out, checkErrors(inner)...)
			}
		}
	default:
		hasCode := func(c checkError) bool { return c.Code != "" }
		if inner := errors.Unwrap(err); inner != nil {
			if unwrapped := checkErrors(inner); slices.ContainsFunc(unwrapped, hasCode) {
				return unwrapped
			}
		}
		out = []checkError{{Message: err.Error()}}
		if d, ok := err.(rego.ErrorDetails); ok {
			out[0].Details = strings.Join(d.Lines(), "\n")
		}
	}
	return out
}
