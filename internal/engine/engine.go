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
	"github.com/open-policy-agent/opa/v1/cover"
	"github.com/open-policy-agent/opa/v1/format"
	"github.com/open-policy-agent/opa/v1/tester"

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
	// Select runs, for each mutant, only the tests that cover its line. When false, every test runs.
	Select        bool
	TimeoutFactor float64
}

// Result is the outcome of one mutant.
type Result struct {
	*mutate.Mutant
	Status Status `json:"status"`
	// PatchOK is false when the patched source does not parse, or parses to the same AST.
	PatchOK        bool          `json:"patchOk"`
	KilledBy       string        `json:"killedBy,omitempty"`
	CoveringTests  []string      `json:"coveringTests,omitempty"`
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
	Tests    []string
	Duration time.Duration
	// testDuration and coverage are keyed by full test name (data.pkg.test_x).
	testDuration map[string]time.Duration
	coverage     map[string]cover.Report
}

// RunBaseline runs every test once, then each test alone with coverage.
func RunBaseline(ctx context.Context, p *project.Project) (*Baseline, error) {
	start := time.Now()
	results, err := runTests(ctx, p, p.Modules(), "", nil)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	b := &Baseline{Duration: time.Since(start), testDuration: map[string]time.Duration{}, coverage: map[string]cover.Report{}}
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
		cov := cover.New()
		if _, err := runTests(ctx, p, p.Modules(), exactly(t), cov); err != nil {
			return nil, fmt.Errorf("coverage of %s: %w", t, err)
		}
		b.coverage[t] = cov.Report(p.Modules())
	}
	return b, nil
}

// CoveringTests returns the tests that evaluate the given line.
func (b *Baseline) CoveringTests(file string, line int) []string {
	var out []string
	for _, t := range b.Tests {
		if b.coverage[t].IsCovered(file, line) {
			out = append(out, t)
		}
	}
	return out
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
	tests := b.Tests
	if opts.Select {
		if len(res.CoveringTests) == 0 {
			// Compile without running any test: a mutant that doesn't compile is stillborn, not uncovered.
			if _, err := runTests(ctx, p, mods, "^$", nil); err != nil {
				res.Status, res.Error = Stillborn, firstLine(err.Error())
				return res
			}
			res.Status = NoCoverage
			return res
		}
		tests = res.CoveringTests
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
	for _, r := range results {
		if r.Fail || r.Error != nil {
			res.Status, res.KilledBy = Killed, fullName(r)
			break
		}
	}
	return res
}

// runTests compiles mods and runs the tests whose full name matches filter (all tests if empty).
func runTests(ctx context.Context, p *project.Project, mods map[string]*ast.Module, filter string, cov *cover.Cover) ([]*tester.Result, error) {
	r := tester.NewRunner().
		SetDefaultRegoVersion(p.RegoVersion).
		SetParallel(1).
		Filter(filter).
		SetModules(mods)
	if cov != nil {
		r.SetCoverageQueryTracer(cov)
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

func fullName(r *tester.Result) string { return r.Package + "." + r.Name }

func exactly(test string) string { return "^" + regexp.QuoteMeta(test) + "$" }

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
