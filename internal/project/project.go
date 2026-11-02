// Package project loads the policies, tests and data of a run, like `opa test` does.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/loader"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

// File is one parsed Rego file.
type File struct {
	Path   string
	Source string
	Module *ast.Module
	// IsTest is true for a test file: its name ends with _test.rego, or it defines a test_ rule. Test files
	// are never mutated, so fixtures and helpers next to the tests are left alone.
	IsTest bool
}

// Project is the set of Rego files and data documents evaluated together.
type Project struct {
	Files       []*File
	RegoVersion ast.RegoVersion
	// Store holds the data documents (data.json, data.yaml and other JSON or YAML files under the paths).
	// Test runs only read it, so one store is shared by all of them.
	Store storage.Store
}

// InvalidError means the files under the paths could not be loaded: a Rego file does not parse, or a data
// file is not valid. The policy cannot be tested as it is, so this is a baseline failure, not a usage error.
type InvalidError struct {
	Err error
	// ParsesAsV0 is set when the Rego files fail to parse as Rego v1 but parse as Rego v0.
	ParsesAsV0 bool
}

func (e *InvalidError) Error() string {
	msg := e.Err.Error()
	if e.ParsesAsV0 {
		msg += "\nhint: the policies parse as Rego v0; run with --v0-compatible"
	}
	return msg
}

func (e *InvalidError) Unwrap() error { return e.Err }

// Load loads the Rego files and data files under paths with the loader that `opa test` uses. A path that
// does not exist is reported as a plain error (a usage error), and invalid files as an *InvalidError.
func Load(paths []string, version ast.RegoVersion) (*Project, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: no such file or directory", path)
		} else if err != nil {
			return nil, err
		}
	}
	res, err := load(paths, version)
	if err != nil {
		inv := &InvalidError{Err: err}
		if version != ast.RegoV0 && hasParseError(err) {
			_, errV0 := load(paths, ast.RegoV0)
			inv.ParsesAsV0 = errV0 == nil
		}
		return nil, inv
	}
	p := &Project{RegoVersion: version, Store: inmem.NewFromObject(res.Documents)}
	for _, f := range res.Modules {
		// f.Name is the path as given, which the parsed locations use. The map key is not: the loader drops
		// the leading "/" of an absolute path there.
		p.Files = append(p.Files, &File{
			Path: f.Name, Source: string(f.Raw), Module: f.Parsed,
			IsTest: strings.HasSuffix(f.Name, "_test.rego") || hasTests(f.Parsed),
		})
	}
	if len(p.Files) == 0 {
		return nil, fmt.Errorf("no .rego file found in %s", strings.Join(paths, ", "))
	}
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	return p, nil
}

func load(paths []string, version ast.RegoVersion) (*loader.Result, error) {
	return loader.NewFileLoader().
		WithRegoVersion(version).
		WithProcessAnnotation(true).
		Filtered(paths, nil)
}

func hasParseError(err error) bool {
	errs, ok := err.(loader.Errors)
	if !ok {
		return false
	}
	for _, e := range errs {
		if ae, ok := e.(*ast.Error); ok && ae.Code == ast.ParseErr {
			return true
		}
	}
	return false
}

// Parse parses one module with the project's Rego version and the loader's options.
func (p *Project) Parse(path, src string) (*ast.Module, error) {
	return ast.ParseModuleWithOpts(path, src, ast.ParserOptions{RegoVersion: p.RegoVersion, ProcessAnnotation: true})
}

// Modules returns the parsed modules keyed by path, as the OPA test runner expects them.
func (p *Project) Modules() map[string]*ast.Module {
	mods := make(map[string]*ast.Module, len(p.Files))
	for _, f := range p.Files {
		mods[f.Path] = f.Module
	}
	return mods
}

func hasTests(mod *ast.Module) bool {
	for _, r := range mod.Rules {
		if strings.HasPrefix(string(r.Head.Name), "test_") {
			return true
		}
		if ref := r.Head.Ref(); len(ref) > 0 {
			if s, ok := ref[0].Value.(ast.Var); ok && strings.HasPrefix(string(s), "test_") {
				return true
			}
		}
	}
	return false
}
