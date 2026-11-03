package engine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/tester"
	"github.com/open-policy-agent/opa/v1/topdown"

	"github.com/adamaKomi/regodrill/internal/mutate"
	"github.com/adamaKomi/regodrill/internal/project"
)

func baseline(t *testing.T, dir string) (*project.Project, *Baseline) {
	t.Helper()
	p, err := project.Load([]string{dir}, ast.RegoV1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunBaseline(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return p, b
}

func TestBaselineNamesDuplicateTestsLikeTheRunner(t *testing.T) {
	_, b := baseline(t, "../../testdata/scenarios/indexed-rule-and-duplicate-tests")
	want := []string{
		"data.svc_test.test_allowed",
		"data.svc_test.test_load_balancer_denied",
		`data.svc_test["test_allowed#01"]`,
	}
	if !slices.Equal(b.Tests, want) {
		t.Fatalf("tests = %q, want %q", b.Tests, want)
	}
	// Every test, the renamed one included, was traced.
	for _, name := range b.Tests {
		if len(b.probes[name].refs) == 0 {
			t.Errorf("%s: no lookup recorded", name)
		}
	}
}

func TestBaselineCoverageAndReach(t *testing.T) {
	_, b := baseline(t, "../../testdata/scenarios/indexed-rule-and-duplicate-tests")
	// Line 7 (input.type == "LoadBalancer") is evaluated only when the indexed input matches.
	file := filepath.FromSlash("../../testdata/scenarios/indexed-rule-and-duplicate-tests/policy.rego")
	if got, want := b.CoveringTests(file, 7), []string{"data.svc_test.test_load_balancer_denied"}; !slices.Equal(got, want) {
		t.Errorf("covering tests of line 7 = %q, want %q", got, want)
	}
	// All three tests look the rule up.
	if got := b.ReachingTests(ast.MustParseRef("data.svc.deny")); len(got) != 3 {
		t.Errorf("tests reaching data.svc.deny = %q, want all 3", got)
	}
	// A lookup of the whole package reaches every rule in it, and vice versa.
	if got := b.ReachingTests(ast.MustParseRef("data.svc")); len(got) != 3 {
		t.Errorf("tests reaching data.svc = %q, want all 3", got)
	}
	if got := b.ReachingTests(ast.MustParseRef("data.other.rule")); len(got) != 0 {
		t.Errorf("tests reaching data.other.rule = %q, want none", got)
	}
}

// Selection must give the same verdicts as running every test (docs/spike-m0.md, finding 2).
func TestSelectionIsSound(t *testing.T) {
	p, b := baseline(t, "../../testdata/scenarios/indexed-rule-and-duplicate-tests")
	mutants := mutate.Generate(p, map[string]bool{mutate.ExprRemove: true})
	selected := Run(context.Background(), p, b, mutants, Options{Parallel: 2, Select: true, TimeoutFactor: 5})
	all := Run(context.Background(), p, b, mutants, Options{Parallel: 2, Select: false, TimeoutFactor: 5})

	want := map[int]struct {
		status   Status
		killedBy string
	}{
		6: {Killed, `data.svc_test["test_allowed#01"]`}, // kind == "Service": only the renamed duplicate catches it
		7: {Killed, "data.svc_test.test_allowed"},       // type == "LoadBalancer": its killer never covers line 7
		// Line 8 (msg := ...) has no mutant: the head needs msg.
	}
	if len(mutants) != len(want) {
		t.Fatalf("got %d mutants, want %d", len(mutants), len(want))
	}
	for i, m := range mutants {
		w := want[m.Line]
		for mode, r := range map[string]*Result{"select": selected[i], "all": all[i]} {
			if r.Status != w.status || r.KilledBy != w.killedBy {
				t.Errorf("%s, line %d: status %s killed by %q, want %s killed by %q", mode, m.Line, r.Status, r.KilledBy, w.status, w.killedBy)
			}
		}
		if m.Rule != "data.svc.deny" {
			t.Errorf("line %d: rule = %q, want data.svc.deny", m.Line, m.Rule)
		}
	}
}

// A test that passes, then fails on the unmodified policy (time, randomness) stops the run: its failures on
// mutants would be false kills.
func TestUnstableBaseline(t *testing.T) {
	p, err := project.Load([]string{"../../testdata/authz"}, ast.RegoV1)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	testRunner = func(ctx context.Context, p *project.Project, mods map[string]*ast.Module, filter string, tracer topdown.QueryTracer) ([]*tester.Result, error) {
		results, err := runTests(ctx, p, mods, filter, tracer)
		calls++
		if calls == 2 {
			for _, r := range results {
				if r.Name == "test_minor_denied" {
					r.Fail = true
				}
			}
		}
		return results, err
	}
	defer func() { testRunner = runTests }()

	_, err = RunBaseline(context.Background(), p)
	var be *BaselineError
	if !errors.As(err, &be) || !be.Unstable || !slices.Equal(be.Failures, []string{"data.authz_test.test_minor_denied"}) {
		t.Fatalf("got %v, want an unstable-test error naming test_minor_denied", err)
	}
	if !strings.Contains(err.Error(), "unstable tests") {
		t.Errorf("message %q", err)
	}
}

// The fast path (fastrun.go) runs plain tests; a project with a parameterized test uses OPA's runner, and a
// test name defined in two files turns test selection off.
func TestRunPathChoice(t *testing.T) {
	cases := []struct {
		dir             string
		fast, ambiguous bool
	}{
		{"../../testdata/authz", true, false},
		{"../../testdata/scenarios/indexed-rule-and-duplicate-tests", true, false}, // duplicates in one file
		{"../../testdata/scenarios/parameterized-tests", false, false},
		{"../../testdata/scenarios/duplicate-tests-across-files", true, true},
	}
	for _, c := range cases {
		_, b := baseline(t, c.dir)
		if b.fast != c.fast || b.ambiguousNames != c.ambiguous {
			t.Errorf("%s: fast %v, ambiguous %v; want %v, %v", c.dir, b.fast, b.ambiguousNames, c.fast, c.ambiguous)
		}
	}
}
