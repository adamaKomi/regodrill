// Package project loads the Rego files of a policy directory.
package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
)

// File is one parsed Rego file.
type File struct {
	Path   string
	Source string
	Module *ast.Module
	// IsTest is true when the file defines at least one test_ rule. Test files are never mutated.
	IsTest bool
}

// Project is the set of Rego files evaluated together.
type Project struct {
	Files       []*File
	RegoVersion ast.RegoVersion
}

// Load parses every .rego file under paths.
func Load(paths []string, version ast.RegoVersion) (*Project, error) {
	p := &Project{RegoVersion: version}
	seen := map[string]bool{}
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".rego" || seen[path] {
				return nil
			}
			seen[path] = true
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			mod, err := p.Parse(path, string(src))
			if err != nil {
				return err
			}
			p.Files = append(p.Files, &File{Path: path, Source: string(src), Module: mod, IsTest: hasTests(mod)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(p.Files) == 0 {
		return nil, fmt.Errorf("no .rego file found in %s", strings.Join(paths, ", "))
	}
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	return p, nil
}

// Parse parses one module with the project's Rego version.
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
