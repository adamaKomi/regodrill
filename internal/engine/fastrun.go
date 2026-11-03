package engine

// A faster way to run a mutant's tests, for the common case.
//
// OPA's test runner attaches a query tracer to every test it evaluates (it keeps the test-case events of
// parameterized tests). An enabled tracer makes the evaluator build an event for every step, which took
// about a third of the CPU time of a run, plus the garbage it leaves. For tests that are plain rules, the
// runner's verdict is simple, and this file evaluates them the same way without a tracer:
//
//   - the modules are compiled like the runner does: same capabilities, print statements enabled, and the
//     same renaming of duplicate test names (rewriteDuplicateTestNames below is the runner's own stage);
//   - each test is the query of its rule: an error is a failure, so is an empty result, and so is any value
//     but true.
//
// Tests run one at a time, in name order, and stop at the first failure: that test is the one the report
// names (the first failing test by name), as with the runner. Parameterized tests (`test_x[name] if ...`)
// are judged on sub-results built from the trace, so a project that has one keeps using the runner.
//
// Duplicate test names are renamed in module path order, so a project with the same test name in two files
// gets stable names (the runner's order changes between compilations).

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"

	"github.com/adamaKomi/regodrill/internal/project"
)

const (
	testPrefix     = "test_"
	skipTestPrefix = "todo_test_"
)

// hasParameterizedTests reports whether a test rule has a ref after its name (`test_x[name] if ...`).
func hasParameterizedTests(mods map[string]*ast.Module) bool {
	for _, mod := range mods {
		for _, rule := range mod.Rules {
			name, ref := testRuleName(rule.Head)
			if strings.HasPrefix(name, testPrefix) && len(rule.Head.Ref()) > len(ref) {
				return true
			}
		}
	}
	return false
}

// errCompile wraps a compile error of the mutated modules: the mutant is stillborn.
type errCompile struct{ err error }

func (e errCompile) Error() string { return e.err.Error() }

// runFast compiles mods and runs the named tests in order until one fails. It returns the failing test, or
// "" when all pass. A compile error is an errCompile; a cancelled or timed-out context is returned as is.
func runFast(ctx context.Context, p *project.Project, mods map[string]*ast.Module, tests []string) (string, error) {
	c := ast.NewCompiler().
		WithEnablePrintStatements(true).
		WithDefaultRegoVersion(p.RegoVersion)
	if p.Capabilities != nil {
		c = c.WithCapabilities(p.Capabilities)
	}
	c.WithStageAfterID(ast.StageRewriteRuleHeadRefs, ast.CompilerStageDefinition{
		Name:       "RewriteDuplicateTestNames",
		MetricName: "rewrite_duplicate_test_names",
		Stage:      rewriteDuplicateTestNames,
	})
	if c.Compile(mods); c.Failed() {
		return "", errCompile{c.Errors}
	}

	// Map the report names (data.pkg.test_x, data.pkg["test_x#01"]) to their queries.
	queries := map[string]string{}
	for _, mod := range c.Modules {
		for _, rule := range mod.Rules {
			name, ref := testRuleName(rule.Head)
			if strings.HasPrefix(name, testPrefix) {
				path := mod.Package.Path.Extend(ref)
				queries[path.String()] = path.String()
			}
		}
	}

	ctx = ast.WithCompiler(ctx, c)
	print := topdown.NewPrintHook(io.Discard)
	for _, test := range tests {
		query, ok := queries[test]
		if !ok {
			// The mutant removed the test's package or renamed it: the runner would not find it either, so it
			// cannot fail. (Mutants never touch test files, so this does not happen in practice.)
			continue
		}
		opts := []func(*rego.Rego){rego.Compiler(c), rego.Query(query), rego.PrintHook(print)}
		if p.Store != nil {
			opts = append(opts, rego.Store(p.Store))
		}
		pq, err := rego.New(opts...).PrepareForEval(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return test, nil // the runner reports this test as an error: it fails
		}
		rs, err := pq.Eval(ctx)
		switch {
		case ctx.Err() != nil:
			return "", ctx.Err()
		case err != nil:
			return test, nil
		case len(rs) == 0:
			return test, nil
		}
		if b, ok := rs[0].Expressions[0].Value.(bool); !ok || !b {
			return test, nil
		}
	}
	return "", nil
}

// testRuleName is the runner's ruleName: the name of a test rule and the ground prefix of its head up to it.
func testRuleName(h *ast.Head) (string, ast.Ref) {
	var n string
	rgp := h.Ref().GroundPrefix()
	i := 0
	for i = range rgp {
		switch v := rgp[i].Value.(type) {
		case ast.Var:
			n = string(v)
		case ast.String:
			n = string(v)
		default:
			n = ""
		}
		if strings.HasPrefix(n, testPrefix) || strings.HasPrefix(n, skipTestPrefix) {
			break
		}
	}
	return n, rgp[:i+1]
}

// rewriteDuplicateTestNames is the runner's compiler stage (OPA v1.21.3, v1/tester/runner.go, Apache 2.0):
// a second test_x in a package becomes test_x#01, and so on. One change: modules are visited in path order,
// not in map order, so that duplicates in two files get the same names in every run.
func rewriteDuplicateTestNames(compiler *ast.Compiler) *ast.Error {
	count := map[string]int{}
	paths := make([]string, 0, len(compiler.Modules))
	for path := range compiler.Modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		mod := compiler.Modules[path]
		for _, rule := range mod.Rules {
			name, ref := testRuleName(rule.Head)
			if !strings.HasPrefix(name, testPrefix) {
				continue
			}
			key := mod.Package.Path.Extend(ref).String()
			if k, ok := count[key]; ok {
				dynamicSuffix := rule.Head.Ref()[len(ref):]
				newName := fmt.Sprintf("%s#%02d", name, k)
				if len(ref) == 1 {
					ref[0] = ast.VarTerm(newName)
				} else {
					ref[len(ref)-1] = ast.StringTerm(newName)
				}
				rule.Head.SetRef(append(ref, dynamicSuffix...))
			}
			count[key]++
		}
	}
	return nil
}
