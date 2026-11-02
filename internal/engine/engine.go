// Package engine runs the policy tests against the original policy and against each mutant.
package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/format"
	"github.com/open-policy-agent/opa/v1/tester"
	"github.com/open-policy-agent/opa/v1/topdown"

	"github.com/adamaKomi/regodrill/internal/mutate"
	"github.com/adamaKomi/regodrill/internal/project"
)

// Status of a mutant, as in docs/mutation-testing-spec.md §2.
type Status string

const (
	Killed     Status = "killed"
	Survived   Status = "survived"
	Timeout    Status = "timeout"
	NoCoverage Status = "no_coverage"
	Stillborn  Status = "stillborn"
)

// Options configure a run.
type Options struct {
	Parallel int
	// Select runs, for each mutant, only the tests that reach its enclosing rule. When false, every test runs.
	Select        bool
	TimeoutFactor float64
}

// Result is the outcome of one mutant.
type Result struct {
	*mutate.Mutant
	Status Status `json:"status"`
	// PatchOK is false when the patched source does not parse, or parses to the same AST.
	PatchOK       bool     `json:"patchOk"`
	KilledBy      string   `json:"killedBy,omitempty"`
	CoveringTests []string `json:"coveringTests,omitempty"`
	// SelectedTests are the tests that reach the enclosing rule: the ones run when selection is on.
	SelectedTests  []string      `json:"selectedTests,omitempty"`
	Error          string        `json:"error,omitempty"`
	Duration       time.Duration `json:"-"`
	DurationMillis float64       `json:"durationMs"`
}

// BaselineError means the unmodified tests do not all pass.
type BaselineError struct{ Failures []string }

func (e *BaselineError) Error() string {
	return "baseline tests failed: " + strings.Join(e.Failures, ", ")
}

// Baseline holds the results of the unmodified test run.
type Baseline struct {
	// Tests are full test names as OPA's test filter sees them: data.pkg.test_x, or data.pkg["test_x#01"] for
	// a duplicate name that the test runner renamed. They are also the names shown in reports.
	Tests    []string
	Duration time.Duration
	// The maps below are keyed by test name.
	testDuration map[string]time.Duration
	probes       map[string]*probe
}

// RunBaseline runs every test once, with a recorder that notes what each test evaluates.
func RunBaseline(ctx context.Context, p *project.Project) (*Baseline, error) {
	start := time.Now()
	rec := &recorder{probes: map[string]*probe{}}
	results, err := runTests(ctx, p, p.Modules(), "", rec)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	b := &Baseline{Duration: time.Since(start), testDuration: map[string]time.Duration{}, probes: map[string]*probe{}}
	var failures []string
	for _, r := range results {
		name := fullName(r)
		if r.Skip {
			continue
		}
		if r.Fail || r.Error != nil {
			failures = append(failures, name)
			continue
		}
		b.Tests = append(b.Tests, name)
		b.testDuration[name] = r.Duration
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return nil, &BaselineError{Failures: failures}
	}
	sort.Strings(b.Tests)
	for _, t := range b.Tests {
		pr, ok := rec.probes[t]
		if !ok {
			// Test names and traced refs both come from the runner, so this is a bug, not a user error.
			return nil, fmt.Errorf("baseline: no trace recorded for %s", t)
		}
		b.probes[t] = pr
	}
	return b, nil
}

// CoveringTests returns the tests that evaluate the given line.
func (b *Baseline) CoveringTests(file string, line int) []string {
	var out []string
	for _, t := range b.Tests {
		if b.probes[t].lines[file][line] {
			out = append(out, t)
		}
	}
	return out
}

// ReachingTests returns the tests that look up the rule at path, or a document that contains it or that it
// contains. Only these tests can see a change to the rule. Line coverage is not enough: rule indexing skips
// the body of a rule whose indexed expressions don't match the input, so the body is not covered, yet removing
// one of those expressions changes the result.
func (b *Baseline) ReachingTests(path ast.Ref) []string {
	var out []string
	for _, t := range b.Tests {
		for _, ref := range b.probes[t].refs {
			if ref.HasPrefix(path) || path.HasPrefix(ref) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// recorder is a query tracer for a whole test run. The runner evaluates each test as its own query, one at a
// time (parallelism 1), and the first lookup of that query is the test rule itself: the recorder starts a new
// probe there, keyed by the test's full name.
type recorder struct {
	probes  map[string]*probe
	current *probe
}

func (*recorder) Enabled() bool               { return true }
func (*recorder) Config() topdown.TraceConfig { return topdown.TraceConfig{} }

func (r *recorder) TraceEvent(e topdown.Event) {
	if e.QueryID == 0 && e.ParentID == 0 {
		switch e.Op {
		case topdown.EnterOp:
			r.current = nil // a new test query; its name comes with the first lookup
			return
		case topdown.IndexOp:
			if r.current == nil && e.Ref != nil {
				name := e.Ref.GroundPrefix().String()
				if r.probes[name] == nil {
					r.probes[name] = newProbe()
				}
				r.current = r.probes[name]
				return
			}
		}
	}
	if r.current != nil {
		r.current.TraceEvent(e)
	}
}

// probe records the lines a test evaluates and the rules it looks up.
type probe struct {
	lines map[string]map[int]bool // file → line → evaluated
	refs  []ast.Ref               // ground prefixes of the documents looked up, without duplicates
	seen  map[string]bool
}

func newProbe() *probe { return &probe{lines: map[string]map[int]bool{}, seen: map[string]bool{}} }

func (pr *probe) TraceEvent(e topdown.Event) {
	switch e.Op {
	case topdown.EvalOp:
		pr.cover(e.Location)
	case topdown.ExitOp:
		if rule, ok := e.Node.(*ast.Rule); ok {
			pr.cover(rule.Head.Location)
		}
	case topdown.IndexOp:
		if e.Ref == nil {
			return
		}
		ref := e.Ref.GroundPrefix()
		if k := ref.String(); !pr.seen[k] {
			pr.seen[k] = true
			pr.refs = append(pr.refs, ref)
		}
	}
}

func (pr *probe) cover(loc *ast.Location) {
	if loc == nil {
		return
	}
	if pr.lines[loc.File] == nil {
		pr.lines[loc.File] = map[int]bool{}
	}
	// An expression may span several lines; its first line is the one a mutant on it reports.
	pr.lines[loc.File][loc.Row] = true
}

// Run evaluates every mutant with a pool of workers. Results are in the order of mutants.
func Run(ctx context.Context, p *project.Project, b *Baseline, mutants []*mutate.Mutant, opts Options) []*Result {
	results := make([]*Result, len(mutants))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range max(1, opts.Parallel) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = evaluate(ctx, p, b, mutants[i], opts)
			}
		}()
	}
	for i := range mutants {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func evaluate(ctx context.Context, p *project.Project, b *Baseline, m *mutate.Mutant, opts Options) *Result {
	start := time.Now()
	res := &Result{Mutant: m}
	defer func() {
		res.Duration = time.Since(start)
		res.DurationMillis = float64(res.Duration.Microseconds()) / 1000
	}()

	var file *project.File
	for _, f := range p.Files {
		if f.Path == m.File {
			file = f
		}
	}
	mod, err := p.Parse(m.File, m.Apply(file.Source))
	if err != nil {
		res.Status, res.Error = Stillborn, firstLine(err.Error())
		return res
	}
	res.PatchOK = !sameAST(file.Module, mod)

	mods := maps.Clone(p.Modules())
	mods[m.File] = mod

	res.CoveringTests = b.CoveringTests(m.File, m.Line)
	res.SelectedTests = b.ReachingTests(m.RuleRef)
	tests := b.Tests
	if opts.Select {
		if len(res.SelectedTests) == 0 {
			// Compile without running any test: a mutant that doesn't compile is stillborn, not uncovered.
			if _, err := runTests(ctx, p, mods, "^$", nil); err != nil {
				res.Status, res.Error = Stillborn, firstLine(err.Error())
				return res
			}
			res.Status = NoCoverage
			return res
		}
		tests = res.SelectedTests
	}

	var budget time.Duration
	for _, t := range tests {
		budget += b.testDuration[t]
	}
	budget = max(2*time.Second, time.Duration(float64(budget)*opts.TimeoutFactor))
	tctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	filter := ""
	if opts.Select {
		filter = anyOf(tests)
	}
	results, err := runTests(tctx, p, mods, filter, nil)
	switch {
	case errors.Is(tctx.Err(), context.DeadlineExceeded):
		res.Status = Timeout
		return res
	case err != nil:
		res.Status, res.Error = Stillborn, firstLine(err.Error())
		return res
	}
	res.Status = Survived
	if len(res.CoveringTests) == 0 {
		// Same verdict with or without selection: a survivor that no test evaluates is uncovered.
		res.Status = NoCoverage
	}
	for _, r := range results {
		if r.Fail || r.Error != nil {
			res.Status, res.KilledBy = Killed, fullName(r)
			break
		}
	}
	return res
}

// runTests compiles mods and runs the tests whose full name matches filter (all tests if empty).
func runTests(ctx context.Context, p *project.Project, mods map[string]*ast.Module, filter string, tracer topdown.QueryTracer) ([]*tester.Result, error) {
	r := tester.NewRunner().
		SetDefaultRegoVersion(p.RegoVersion).
		SetParallel(1).
		Filter(filter).
		SetModules(mods)
	if tracer != nil {
		// Not a *cover.Cover, so the runner adds no supplementary passes with indexing or early exit disabled.
		// Parallelism is 1, so the tracer sees one test at a time.
		r.SetCoverageQueryTracer(tracer)
	}
	ch, err := r.RunTests(ctx, nil)
	if err != nil {
		return nil, err
	}
	var out []*tester.Result
	for res := range ch {
		out = append(out, res)
	}
	return out, ctx.Err()
}

// fullName is the name that the runner's filter matches: the package path extended with the test name. A
// renamed duplicate (test_x#01) is not a valid identifier, so it prints as data.pkg["test_x#01"]. Like
// ast.Ref.Extend in the runner, the name is a string term.
func fullName(r *tester.Result) string {
	pkg, err := ast.ParseRef(r.Package)
	if err != nil {
		return r.Package + "." + r.Name
	}
	return pkg.Append(ast.StringTerm(r.Name)).String()
}

func anyOf(tests []string) string {
	quoted := make([]string, len(tests))
	for i, t := range tests {
		quoted[i] = regexp.QuoteMeta(t)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func sameAST(a, b *ast.Module) bool {
	fa, errA := format.Ast(a)
	fb, errB := format.Ast(b)
	return errA == nil && errB == nil && string(fa) == string(fb)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
