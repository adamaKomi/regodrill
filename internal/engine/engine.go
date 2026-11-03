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
	// Ignored mutants are not run: an annotation or the configuration file says a person checked them.
	Ignored Status = "ignored"
	// Equivalent mutants parse to the same AST as the original (trivial compiler equivalence): no test can
	// kill them, so they are not run and not scored.
	Equivalent Status = "equivalent"
)

// Options configure a run.
type Options struct {
	Parallel int
	// Select runs, for each mutant, only the tests that reach its enclosing rule. When false, every test runs.
	Select        bool
	TimeoutFactor float64
	// Progress, when set, is called after each mutant with the number done and the total. Calls come from
	// several goroutines, one at a time.
	Progress func(done, total int)
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
	SelectedTests []string `json:"selectedTests,omitempty"`
	IgnoreReason  string   `json:"ignoreReason,omitempty"`
	// Message explains the result in one sentence, for reports and editors.
	Message        string        `json:"message,omitempty"`
	Error          string        `json:"error,omitempty"`
	Duration       time.Duration `json:"-"`
	DurationMillis float64       `json:"durationMs"`
}

// BaselineError means the unmodified tests do not all pass, that some pass only sometimes, or that there is
// no test to run.
type BaselineError struct {
	Failures []string
	// Unstable is set when the tests passed once and failed on the second run.
	Unstable bool
}

func (e *BaselineError) Error() string {
	switch {
	case len(e.Failures) == 0:
		return "no test found: mutation testing needs tests that pass on the unmodified policy"
	case e.Unstable:
		return "unstable tests: they passed, then failed on the unmodified policy (time, randomness?): " + strings.Join(e.Failures, ", ")
	}
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
	// fast is true when mutants can run without the test runner (see fastrun.go): no parameterized test.
	fast bool
	// ambiguousNames is true when two test files of one package define the same test name. The runner
	// renames the second one test_x#01, but in an order that can change between compilations, so a name
	// may not designate the same test in the baseline and in a mutant run: test selection is turned off.
	// The fast path names them in a stable order, so killedBy stays the same from run to run; with
	// parameterized tests too, the runner decides and killedBy can vary between such duplicates.
	ambiguousNames bool
}

// duplicateTestsAcrossFiles reports whether a test name of a package is defined in more than one file.
func duplicateTestsAcrossFiles(mods map[string]*ast.Module) bool {
	files := map[string]string{}
	for path, mod := range mods {
		for _, rule := range mod.Rules {
			name, ref := testRuleName(rule.Head)
			if !strings.HasPrefix(name, testPrefix) {
				continue
			}
			key := mod.Package.Path.Extend(ref).String()
			if f, ok := files[key]; ok && f != path {
				return true
			}
			files[key] = path
		}
	}
	return false
}

// RunBaseline runs every test once, with a recorder that notes what each test evaluates.
func RunBaseline(ctx context.Context, p *project.Project) (*Baseline, error) {
	start := time.Now()
	rec := &recorder{probes: map[string]*probe{}}
	results, err := testRunner(ctx, p, p.Modules(), "", rec)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	b := &Baseline{
		Duration: time.Since(start), testDuration: map[string]time.Duration{}, probes: map[string]*probe{},
		fast: !hasParameterizedTests(p.Modules()), ambiguousNames: duplicateTestsAcrossFiles(p.Modules()),
	}
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
	if len(b.Tests) == 0 {
		return nil, &BaselineError{}
	}
	sort.Strings(b.Tests)
	// Run the tests a second time: a test that depends on time or randomness would turn some mutants into
	// false kills. Results that differ stop the run (docs/mutation-testing-spec.md §10).
	again, err := testRunner(ctx, p, p.Modules(), "", nil)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	var unstable []string
	for _, r := range again {
		if !r.Skip && (r.Fail || r.Error != nil) {
			unstable = append(unstable, fullName(r))
		}
	}
	if len(unstable) > 0 {
		sort.Strings(unstable)
		return nil, &BaselineError{Failures: unstable, Unstable: true}
	}
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
	var mu sync.Mutex
	done := 0
	for range max(1, opts.Parallel) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = evaluate(ctx, p, b, mutants[i], opts)
				if opts.Progress != nil {
					mu.Lock()
					done++
					opts.Progress(done, len(mutants))
					mu.Unlock()
				}
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
	if !res.PatchOK {
		res.Status = Equivalent
		return res
	}

	mods := maps.Clone(p.Modules())
	mods[m.File] = mod

	res.CoveringTests = b.CoveringTests(m.File, m.Line)
	res.SelectedTests = b.ReachingTests(m.RuleRef)
	tests := b.Tests
	if b.ambiguousNames {
		opts.Select = false
	}
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

	if b.fast {
		killer, err := runFast(tctx, p, mods, tests)
		var ce errCompile
		switch {
		case errors.Is(tctx.Err(), context.DeadlineExceeded):
			res.Status = Timeout
		case errors.As(err, &ce):
			res.Status, res.Error = Stillborn, firstLine(ce.Error())
		case err != nil:
			res.Status, res.Error = Stillborn, firstLine(err.Error())
		case killer != "":
			res.Status, res.KilledBy = Killed, killer
		case len(res.CoveringTests) == 0:
			res.Status = NoCoverage
		default:
			res.Status = Survived
		}
		return res
	}

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
	// The runner's test order is not deterministic, so report the first failing test by name.
	for _, r := range results {
		if r.Fail || r.Error != nil {
			if name := fullName(r); res.Status != Killed || name < res.KilledBy {
				res.Status, res.KilledBy = Killed, name
			}
		}
	}
	return res
}

// testRunner runs the baseline's tests. Tests replace it to simulate an unstable test.
var testRunner = runTests

// runTests compiles mods and runs the tests whose full name matches filter (all tests if empty).
func runTests(ctx context.Context, p *project.Project, mods map[string]*ast.Module, filter string, tracer topdown.QueryTracer) ([]*tester.Result, error) {
	r := tester.NewRunner().
		SetDefaultRegoVersion(p.RegoVersion).
		SetParallel(1).
		Filter(filter).
		SetModules(mods)
	if p.Store != nil {
		r.SetStore(p.Store)
	}
	// The runner stops each test after its own timeout (5 s by default, like opa test) and reports an error.
	// When a mutant's budget is longer, let the budget decide, so that a slow mutant is a timeout.
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline) + time.Second; d > 5*time.Second {
			r.SetTimeout(d)
		}
	}
	if p.Capabilities != nil {
		// The same compiler the runner would build, with the project's capabilities.
		r.SetCompiler(ast.NewCompiler().
			WithCapabilities(p.Capabilities).
			WithEnablePrintStatements(true).
			WithDefaultRegoVersion(p.RegoVersion))
	}
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

// sameAST reports whether two parsed modules are the same policy: OPA's comparison ignores locations and
// comments, and compares sets as sets, so {"b", "a"} equals {"a", "b", "a"}. (Comparing formatted text
// would not: the formatter keeps the order in which set elements were written.)
func sameAST(a, b *ast.Module) bool {
	return a.Equal(b)
}

// firstLine summarizes a compiler error on one line. OPA lists several errors as "N errors occurred:"
// followed by one per line: keep the count and the first error, which is the useful part.
func firstLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 1 && strings.HasSuffix(lines[0], "errors occurred:") {
		return lines[0] + " " + strings.TrimSpace(lines[1])
	}
	return lines[0]
}
