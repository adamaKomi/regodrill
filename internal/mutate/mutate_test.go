package mutate

import (
	"slices"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/adamaKomi/regodrill/internal/project"
)

// patched generates the mutants of one policy file and returns each patched source, without the package line.
func patched(t *testing.T, version ast.RegoVersion, src string, ops ...string) []string {
	t.Helper()
	p := &project.Project{RegoVersion: version}
	mod, err := p.Parse("policy.rego", src)
	if err != nil {
		t.Fatal(err)
	}
	p.Files = []*project.File{{Path: "policy.rego", Source: src, Module: mod}}
	enabled := map[string]bool{}
	for _, op := range ops {
		enabled[op] = true
	}
	var out []string
	for _, m := range Generate(p, enabled) {
		got := m.Apply(src)
		if got == src {
			t.Errorf("%s at %d:%d changes nothing", m.Operator, m.Line, m.Column)
		}
		out = append(out, strings.TrimPrefix(got, "package p\n\n"))
	}
	return out
}

func check(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("got %d mutants:\n%s\nwant %d:\n%s", len(got), strings.Join(got, "---\n"), len(want), strings.Join(want, "---\n"))
	}
}

func TestCompNegateAndBoundary(t *testing.T) {
	src := "package p\n\nallow if input.a >= -2\n"
	check(t, patched(t, ast.RegoV1, src, CompBoundary, CompNegate), []string{
		"allow if input.a > -2\n",
		"allow if input.a < -2\n",
	})
}

func TestConstNumberNegative(t *testing.T) {
	// The location of a negative number is only "-": the patch must cover the whole token.
	src := "package p\n\nallow if input.a > -12\n"
	check(t, patched(t, ast.RegoV1, src, ConstNumber), []string{
		"allow if input.a > -11\n",
		"allow if input.a > -13\n",
	})
}

func TestConstStringKinds(t *testing.T) {
	src := "package p\n\nallow if input.a == \"x\\\"y\"\n\nraw if input.b == `c:\\d`\n\nin_list if \"admin\" in input.roles\n"
	check(t, patched(t, ast.RegoV1, src, ConstString), []string{
		"allow if input.a == \"x\\\"y_regodrill\"\n\nraw if input.b == `c:\\d`\n\nin_list if \"admin\" in input.roles\n",
		"allow if input.a == \"x\\\"y\"\n\nraw if input.b == `c:\\d_regodrill`\n\nin_list if \"admin\" in input.roles\n",
		"allow if input.a == \"x\\\"y\"\n\nraw if input.b == `c:\\d`\n\nin_list if \"admin_regodrill\" in input.roles\n",
	})
}

func TestNotToggle(t *testing.T) {
	src := "package p\n\ndefault d := false\n\nv := input.x > 1\n\nallow if {\n\tnot input.blocked\n\tx := input.a\n\tx > 1\n}\n"
	check(t, patched(t, ast.RegoV1, src, NotToggle), []string{
		"default d := false\n\nv := input.x > 1\n\nallow if {\n\tinput.blocked\n\tx := input.a\n\tx > 1\n}\n",
		"default d := false\n\nv := input.x > 1\n\nallow if {\n\tnot input.blocked\n\tx := input.a\n\tnot x > 1\n}\n",
	})
}

func TestCollectionDrop(t *testing.T) {
	src := "package p\n\nroles := {\n\t\"a\",\n\t\"b\",\n}\n\nports := [1, -2, 3]\n"
	check(t, patched(t, ast.RegoV1, src, CollectionDrop), []string{
		"roles := {\n\t\"b\",\n}\n\nports := [1, -2, 3]\n",
		"roles := {\n\t\"a\",\n}\n\nports := [1, -2, 3]\n",
		"roles := {\n\t\"a\",\n\t\"b\",\n}\n\nports := [-2, 3]\n",
		"roles := {\n\t\"a\",\n\t\"b\",\n}\n\nports := [1, 3]\n",
		"roles := {\n\t\"a\",\n\t\"b\",\n}\n\nports := [1, -2]\n",
	})
}

func TestQuantWeaken(t *testing.T) {
	src := "package p\n\nok if {\n\tevery k, v in input.m {\n\t\tv > 0\n\t\tk != \"\"\n\t}\n}\n"
	check(t, patched(t, ast.RegoV1, src, QuantWeaken), []string{
		"ok if {\n\tsome k, v in input.m; v > 0; k != \"\"\n}\n",
	})
}

func TestRuleRemove(t *testing.T) {
	src := "package p\n\nallow if input.a\n\nallow if input.b # second\n\nsingle if input.c\n"
	check(t, patched(t, ast.RegoV1, src, RuleRemove), []string{
		"\nallow if input.b # second\n\nsingle if input.c\n",
		"allow if input.a\n\n\nsingle if input.c\n",
	})
}

func TestRuleRemoveV0Chains(t *testing.T) {
	src := "package p\n\nf(x) = y {\n\tx > 1\n\ty := 1\n} {\n\tx <= 1\n\ty := 2\n}\n\ng(\"a\", _)\n\ng(\"b\", z) {\n\tz > 0\n}\n"
	check(t, patched(t, ast.RegoV0, src, RuleRemove), []string{
		// The first block goes; the head takes the second.
		"f(x) = y {\n\tx <= 1\n\ty := 2\n}\n\ng(\"a\", _)\n\ng(\"b\", z) {\n\tz > 0\n}\n",
		// The second block goes.
		"f(x) = y {\n\tx > 1\n\ty := 1\n}\n\ng(\"a\", _)\n\ng(\"b\", z) {\n\tz > 0\n}\n",
		// A definition without a body: the whole line, arguments included.
		"f(x) = y {\n\tx > 1\n\ty := 1\n} {\n\tx <= 1\n\ty := 2\n}\n\n\ng(\"b\", z) {\n\tz > 0\n}\n",
		"f(x) = y {\n\tx > 1\n\ty := 1\n} {\n\tx <= 1\n\ty := 2\n}\n\ng(\"a\", _)\n\n",
	})
}

func TestCRLFAndUnicodeColumns(t *testing.T) {
	src := "package p\r\n\r\nallow if { \"é✓\" != input.x; input.n >= 3 }\r\n"
	p := &project.Project{RegoVersion: ast.RegoV1}
	mod, err := p.Parse("policy.rego", src)
	if err != nil {
		t.Fatal(err)
	}
	p.Files = []*project.File{{Path: "policy.rego", Source: src, Module: mod}}
	ms := Generate(p, map[string]bool{CompBoundary: true})
	if len(ms) != 1 {
		t.Fatalf("got %d mutants, want 1", len(ms))
	}
	m := ms[0]
	if got := m.Apply(src); got != "package p\r\n\r\nallow if { \"é✓\" != input.x; input.n > 3 }\r\n" {
		t.Errorf("patched source %q", got)
	}
	// `allow if { "é✓" != input.x; input.n ` is 39 bytes but 36 UTF-16 code units (é and ✓ are one unit
	// each), so `>=` starts at column 37, not at byte column 40.
	if m.Line != 3 || m.Column != 37 || m.Range.End.Column != 39 {
		t.Errorf("position %d:%d-%d, want 3:37-39", m.Line, m.Column, m.Range.End.Column)
	}
}

func TestExprRemoveSkipsNeededDeclarations(t *testing.T) {
	src := "package p\n\nallow if {\n\tx := input.a\n\tunused := input.b\n\tx > 1\n}\n"
	check(t, patched(t, ast.RegoV1, src, ExprRemove), []string{
		"allow if {\n\tx := input.a\n\t\n\tx > 1\n}\n",
		"allow if {\n\tx := input.a\n\tunused := input.b\n\t\n}\n",
	})
}

func TestInlineIgnores(t *testing.T) {
	src := "package p\n\nallow if {\n\tinput.a >= 1 # regodrill:ignore COMP_BOUNDARY -- checked\n\t# regodrill:ignore-next-line\n\tinput.b >= 2\n}\n\n# regodrill:ignore\nx := 1\n"
	p := &project.Project{RegoVersion: ast.RegoV1}
	mod, err := p.Parse("policy.rego", src)
	if err != nil {
		t.Fatal(err)
	}
	p.Files = []*project.File{{Path: "policy.rego", Source: src, Module: mod}}
	enabled := map[string]bool{CompBoundary: true, ExprRemove: true}
	ignores, warnings := InlineIgnores(p)
	if len(ignores) != 3 || len(warnings) != 0 {
		t.Fatalf("got %d ignores, warnings %v", len(ignores), warnings)
	}
	run, ignored, reasons, stale := ApplyIgnores(p, Generate(p, enabled), ignores, enabled)
	var got []string
	for _, m := range ignored {
		got = append(got, m.Operator+"@"+strings.TrimSpace(m.Original)+":"+reasons[m])
	}
	want := []string{"COMP_BOUNDARY@>=:checked", "EXPR_REMOVE@input.b >= 2:", "COMP_BOUNDARY@>=:"}
	if !slices.Equal(got, want) {
		t.Errorf("ignored %q, want %q", got, want)
	}
	if len(run) != 1 || run[0].Original != "input.a >= 1" {
		t.Errorf("run %v, want only the removal of input.a >= 1", run)
	}
	if len(stale) != 1 || !strings.Contains(stale[0], "policy.rego:9") {
		t.Errorf("stale warnings %q, want one for line 9", stale)
	}
}

// A local declared with := shadows a rule of the same name (here a test helper in the same package):
// removing the declaration makes the body refer to the rule, which cannot compile.
func TestExprRemoveShadowedRuleName(t *testing.T) {
	src := "package p\n\nhpa(x) := x\n\nallow if {\n\thpa := input.object\n\thpa.kind == \"HPA\"\n}\n"
	got := patched(t, ast.RegoV1, src, ExprRemove)
	check(t, got, []string{"hpa(x) := x\n\nallow if {\n\thpa := input.object\n\t\n}\n"})
}
