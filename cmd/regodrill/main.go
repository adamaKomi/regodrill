// Command regodrill runs mutation testing on Rego policies.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	opaversion "github.com/open-policy-agent/opa/v1/version"

	"github.com/adamaKomi/regodrill/internal/engine"
	"github.com/adamaKomi/regodrill/internal/mutate"
	"github.com/adamaKomi/regodrill/internal/project"
)

// Exit codes, as in docs/mutation-testing-spec.md §4.
const (
	exitOK        = 0
	exitUsage     = 1
	exitBaseline  = 2
	exitThreshold = 3
)

const usage = `regodrill: mutation testing for Rego policies

Usage:
  regodrill run [flags] [paths...]    run mutation testing (default path: .)
  regodrill list [flags] [paths...]   list the mutants, without running tests
  regodrill operators [--format json] describe the mutation operators
  regodrill check [flags] <paths...>  parse and compile Rego files, like opa check
  regodrill version                   print the versions of regodrill and OPA

Run "regodrill run -h" for the flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "run":
		return runCommand(args[1:], stdout, stderr)
	case "list":
		return listCommand(args[1:], stdout, stderr)
	case "operators":
		return operatorsCommand(args[1:], stdout, stderr)
	case "check":
		return checkCommand(args[1:], stderr)
	case "version", "--version":
		fmt.Fprintf(stdout, "regodrill %s (OPA %s)\n", version, opaversion.Version)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "regodrill: unknown command %q\n\n%s", args[0], usage)
	return exitUsage
}

// settings of a run or a listing, after merging flags and the configuration file.
type settings struct {
	paths         []string
	enabled       map[string]bool
	v0            bool
	parallel      int
	timeoutFactor float64
	noSelect      bool
	threshold     *float64
	since         string
	allowNetwork  bool
	format        string
	output        string
	ignoreFiles   []string
	cfg           *config
}

// listFlag is a flag that can be repeated.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// parseSettings parses the flags of `run` or `list`. Flags take precedence over the configuration file.
func parseSettings(cmd string, args []string, stderr io.Writer) (*settings, bool) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	formats := "text, json or sarif"
	if cmd == "list" {
		formats = "text or json"
	}
	format := fs.String("format", "text", "report format: "+formats)
	operators := fs.String("operators", "", "comma-separated operators to enable (default: all)")
	excludeOps := fs.String("exclude-operators", "", "comma-separated operators to disable")
	v0 := fs.Bool("v0-compatible", false, "parse policies as Rego v0")
	configPath := fs.String("config", "", "configuration file (default: "+defaultConfig+" if present)")
	since := fs.String("since", "", "only mutate the lines added or changed since this git reference")
	var ignoreFiles listFlag
	fs.Var(&ignoreFiles, "ignore", "skip files and directories whose name matches this pattern when loading, like opa test (repeatable)")
	var parallel *int
	var timeoutFactor, threshold *float64
	var noSelect, allowNetwork *bool
	var output *string
	if cmd == "run" {
		parallel = fs.Int("parallel", runtime.NumCPU(), "mutants evaluated in parallel")
		timeoutFactor = fs.Float64("timeout-factor", 5, "a mutant times out after this many times the baseline duration of its tests (minimum 2 s)")
		noSelect = fs.Bool("no-select", false, "run every test for every mutant, instead of the tests that reach the mutated rule")
		threshold = fs.Float64("threshold", -1, "exit with code 3 if the score is below this value (0 to 100)")
		allowNetwork = fs.Bool("allow-network", false, "keep the built-ins that reach the network (http.send, net.lookup_ip_addr)")
		output = fs.String("output", "", "write the report to this file; the text summary goes to stderr")
	}
	if err := fs.Parse(args); err != nil {
		return nil, false
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	fail := func(format string, a ...any) (*settings, bool) {
		fmt.Fprintf(stderr, "error: "+format+"\n", a...)
		return nil, false
	}

	s := &settings{format: *format, since: *since, v0: *v0}
	switch {
	case s.format == "text" || s.format == "json":
	case s.format == "sarif" && cmd == "run":
	default:
		return fail("unknown format %q: use %s", s.format, formats)
	}

	path, explicit := defaultConfig, false
	if set["config"] {
		path, explicit = *configPath, true
	}
	cfg, err := loadConfig(path, explicit)
	if err != nil {
		return fail("%v", err)
	}
	s.cfg = cfg

	s.paths = fs.Args()
	if len(s.paths) == 0 && cfg != nil && len(cfg.Paths) > 0 {
		s.paths = cfg.paths()
	}
	if len(s.paths) == 0 {
		s.paths = []string{"."}
	}
	if !set["v0-compatible"] && cfg != nil && cfg.V0Compatible != nil {
		s.v0 = *cfg.V0Compatible
	}
	s.ignoreFiles = ignoreFiles
	if !set["ignore"] && cfg != nil {
		s.ignoreFiles = cfg.IgnoreFiles
	}
	for _, pattern := range s.ignoreFiles {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return fail("--ignore %q: %v", pattern, err)
		}
	}

	include := mutate.Operators
	switch {
	case set["operators"]:
		include = splitList(*operators)
	case cfg != nil && len(cfg.Operators.Include) > 0:
		include = cfg.Operators.Include
	}
	var exclude []string
	switch {
	case set["exclude-operators"]:
		exclude = splitList(*excludeOps)
	case cfg != nil:
		exclude = cfg.Operators.Exclude
	}
	s.enabled = map[string]bool{}
	for _, id := range include {
		if !isOperator(id) {
			return fail("unknown operator %q (see regodrill operators)", id)
		}
		s.enabled[id] = true
	}
	for _, id := range exclude {
		if !isOperator(id) {
			return fail("unknown operator %q (see regodrill operators)", id)
		}
		delete(s.enabled, id)
	}
	if len(s.enabled) == 0 {
		return fail("no operator enabled")
	}

	if cmd != "run" {
		return s, true
	}
	s.parallel, s.timeoutFactor, s.noSelect, s.allowNetwork, s.output = *parallel, *timeoutFactor, *noSelect, *allowNetwork, *output
	if cfg != nil {
		if !set["parallel"] && cfg.Parallel != nil {
			s.parallel = *cfg.Parallel
		}
		if !set["timeout-factor"] && cfg.TimeoutFactor != nil {
			s.timeoutFactor = *cfg.TimeoutFactor
		}
		if !set["allow-network"] && cfg.AllowNetwork != nil {
			s.allowNetwork = *cfg.AllowNetwork
		}
		if !set["threshold"] && cfg.Threshold != nil {
			s.threshold = cfg.Threshold
		}
	}
	if set["threshold"] {
		if *threshold < 0 || *threshold > 100 {
			return fail("--threshold must be between 0 and 100")
		}
		s.threshold = threshold
	}
	if s.parallel < 1 {
		return fail("--parallel must be at least 1")
	}
	if s.timeoutFactor <= 0 {
		return fail("--timeout-factor must be positive")
	}
	return s, true
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// plan is what a run will do: the mutants to run, the ignored ones, and the warnings found on the way.
type plan struct {
	project  *project.Project
	all      []*mutate.Mutant // in report order
	run      []*mutate.Mutant
	ignored  map[*mutate.Mutant]string // reason
	warnings []string
}

// prepare loads the project and generates the mutants. It returns an exit code when it fails.
func prepare(s *settings, stderr io.Writer) (*plan, int) {
	version := ast.RegoV1
	if s.v0 {
		version = ast.RegoV0
	}
	p, err := project.Load(s.paths, version, s.ignoreFiles...)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		// Files that don't parse or load mean the baseline cannot compile; a missing path is a usage error.
		var invalid *project.InvalidError
		if errors.As(err, &invalid) {
			return nil, exitBaseline
		}
		return nil, exitUsage
	}
	if s.allowNetwork {
		p.AllowNetwork()
	}

	mutants := mutate.Generate(p, s.enabled)
	if s.cfg != nil && len(s.cfg.Exclude) > 0 {
		kept := mutants[:0]
		for _, m := range mutants {
			if !s.cfg.excluded(m.File) {
				kept = append(kept, m)
			}
		}
		mutants = kept
	}

	ignores, warnings := mutate.InlineIgnores(p)
	if s.cfg != nil {
		ignores = append(ignores, s.cfg.ignores()...)
	}
	// Ignores apply before --since, so that an annotation on an unchanged line is not reported as stale.
	toRun, ignored, reasons, stale := mutate.ApplyIgnores(p, mutants, ignores, s.enabled)
	warnings = append(warnings, stale...)

	if s.since != "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return nil, exitUsage
		}
		changed, err := changedSince(cwd, s.since)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return nil, exitUsage
		}
		keep := func(ms []*mutate.Mutant) []*mutate.Mutant {
			var out []*mutate.Mutant
			for _, m := range ms {
				if changed.contains(m.File, m.Line) {
					out = append(out, m)
				}
			}
			return out
		}
		toRun, ignored = keep(toRun), keep(ignored)
		mutants = keep(mutants)
	}
	pl := &plan{project: p, all: mutants, run: toRun, ignored: map[*mutate.Mutant]string{}, warnings: warnings}
	for _, m := range ignored {
		pl.ignored[m] = reasons[m]
	}
	return pl, exitOK
}

func runCommand(args []string, stdout, stderr io.Writer) int {
	s, ok := parseSettings("run", args, stderr)
	if !ok {
		return exitUsage
	}
	pl, code := prepare(s, stderr)
	if code != exitOK {
		return code
	}
	for _, w := range pl.warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}

	ctx := context.Background()
	t := timing{start: time.Now()}
	// The baseline runs even without mutants to run: a broken test suite must never look like a success.
	baseline, err := engine.RunBaseline(ctx, pl.project)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if hint := baselineHint(err); hint != "" {
			fmt.Fprintln(stderr, "hint:", hint)
		}
		return exitBaseline
	}
	t.baseline, t.tests = baseline.Duration, len(baseline.Tests)
	t.coverageDone = time.Now()
	byMutant := map[*mutate.Mutant]*engine.Result{}
	opts := engine.Options{Parallel: s.parallel, Select: !s.noSelect, TimeoutFactor: s.timeoutFactor}
	if isTerminal(stderr) && len(pl.run) > 0 {
		// Progress on one rewritten line, only for people: CI logs and redirected output stay clean.
		opts.Progress = func(done, total int) {
			fmt.Fprintf(stderr, "\r%d/%d mutants", done, total)
			if done == total {
				fmt.Fprint(stderr, "\r\033[K")
			}
		}
	}
	for _, r := range engine.Run(ctx, pl.project, baseline, pl.run, opts) {
		byMutant[r.Mutant] = r
	}
	t.end = time.Now()

	results := make([]*engine.Result, 0, len(pl.all))
	for _, m := range pl.all {
		if r, ok := byMutant[m]; ok {
			results = append(results, r)
		} else if reason, ok := pl.ignored[m]; ok {
			results = append(results, &engine.Result{Mutant: m, Status: engine.Ignored, IgnoreReason: reason})
		}
	}
	rep := newReport(results, t, s.parallel, !s.noSelect)
	rep.Warnings = pl.warnings
	rep.Summary.Threshold = s.threshold
	rep.Summary.Since = s.since

	out := stdout
	if s.output != "" {
		f, err := os.Create(s.output)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
		defer f.Close()
		out = f
	}
	switch s.format {
	case "json":
		err = writeJSON(out, rep)
	case "sarif":
		err = writeSARIF(out, rep)
	default:
		writeText(out, rep, useColor(out))
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}
	if s.output != "" {
		writeSummary(stderr, rep, useColor(stderr))
	}

	if s.threshold != nil && rep.Summary.Score != nil && *rep.Summary.Score < *s.threshold {
		fmt.Fprintf(stderr, "error: mutation score %.1f %% is below the threshold of %g %%\n", *rep.Summary.Score, *s.threshold)
		return exitThreshold
	}
	return exitOK
}

// undefinedFunction finds the names in "undefined function NAME" compiler errors.
var undefinedFunction = regexp.MustCompile(`undefined function ([A-Za-z_][\w.]*)`)

// baselineHint explains a baseline that fails to compile because of a missing built-in: a network built-in
// that regodrill turns off, or a function that is not an OPA built-in at all (a custom OPA build).
func baselineHint(err error) string {
	var custom []string
	seen := map[string]bool{}
	for _, m := range undefinedFunction.FindAllStringSubmatch(err.Error(), -1) {
		name := m[1]
		if project.IsNetworkBuiltin(name) {
			return fmt.Sprintf("the policy calls %s, which regodrill disables by default so that mutants cannot send requests; run with --allow-network", name)
		}
		if !seen[name] && !strings.HasPrefix(name, "data.") {
			seen[name] = true
			custom = append(custom, name)
		}
	}
	if len(custom) == 0 {
		return ""
	}
	what := "is not an OPA built-in"
	if len(custom) > 1 {
		what = "are not OPA built-ins"
	}
	return fmt.Sprintf("%s %s. If the project runs its tests with a custom OPA build that adds built-ins, regodrill cannot evaluate them yet: run it on the policies that don't call them",
		strings.Join(custom, ", "), what)
}

func listCommand(args []string, stdout, stderr io.Writer) int {
	s, ok := parseSettings("list", args, stderr)
	if !ok {
		return exitUsage
	}
	pl, code := prepare(s, stderr)
	if code != exitOK {
		return code
	}
	for _, w := range pl.warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	type listed struct {
		*mutate.Mutant
		Ignored      bool   `json:"ignored,omitempty"`
		IgnoreReason string `json:"ignoreReason,omitempty"`
	}
	var items []listed
	for _, m := range pl.all {
		reason, ignored := pl.ignored[m]
		items = append(items, listed{m, ignored, reason})
	}
	if s.format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			SchemaVersion int      `json:"schemaVersion"`
			Mutants       []listed `json:"mutants"`
			Warnings      []string `json:"warnings,omitempty"`
		}{1, items, pl.warnings}); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
		return exitOK
	}
	for _, it := range items {
		suffix := ""
		if it.Ignored {
			suffix = "  (ignored"
			if it.IgnoreReason != "" {
				suffix += ": " + it.IgnoreReason
			}
			suffix += ")"
		}
		fmt.Fprintf(stdout, "%s:%d:%d  %-15s %s%s\n", it.File, it.Line, it.Column, it.Operator, it.Change(), suffix)
	}
	fmt.Fprintf(stdout, "\n%d mutants, %d ignored\n", len(items), len(pl.ignored))
	return exitOK
}

func operatorsCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("operators", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(mutate.Descriptions); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
	case "text":
		for _, d := range mutate.Descriptions {
			fmt.Fprintf(stdout, "%-16s %s\n%-16s example: %s\n%-16s risk: %s\n\n", d.ID, d.Summary, "", d.Example, "", d.Risk)
		}
	default:
		fmt.Fprintf(stderr, "error: unknown format %q: use text or json\n", *format)
		return exitUsage
	}
	return exitOK
}
