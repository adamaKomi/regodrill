package engine

import (
	"context"
	"slices"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"

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
	_, b := baseline(t, "../../testdata/selection")
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
	_, b := baseline(t, "../../testdata/selection")
	// Line 7 (input.type == "LoadBalancer") is evaluated only when the indexed input matches.
	if got, want := b.CoveringTests("../../testdata/selection/policy.rego", 7), []string{"data.svc_test.test_load_balancer_denied"}; !slices.Equal(got, want) {
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
	p, b := baseline(t, "../../testdata/selection")
	mutants := mutate.Generate(p, map[string]bool{mutate.ExprRemove: true})
	selected := Run(context.Background(), p, b, mutants, Options{Parallel: 2, Select: true, TimeoutFactor: 5})
	all := Run(context.Background(), p, b, mutants, Options{Parallel: 2, Select: false, TimeoutFactor: 5})

	want := map[int]struct {
		status   Status
		killedBy string
	}{
		6: {Killed, `data.svc_test["test_allowed#01"]`}, // kind == "Service": only the renamed duplicate catches it
		7: {Killed, "data.svc_test.test_allowed"},       // type == "LoadBalancer": its killer never covers line 7
		8: {Stillborn, ""},                              // msg := ...: msg is unsafe in the head
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
