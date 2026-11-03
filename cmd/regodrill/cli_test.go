package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestUsageErrors(t *testing.T) {
	dir := "../../testdata/scenarios/expression-removal"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no command", nil, "Usage:"},
		{"unknown command", []string{"frobnicate"}, `unknown command "frobnicate"`},
		{"unknown operator", []string{"run", "--operators=NOPE", dir}, `unknown operator "NOPE"`},
		{"unknown excluded operator", []string{"run", "--exclude-operators=NOPE", dir}, `unknown operator "NOPE"`},
		{"all operators excluded", []string{"run", "--operators=EXPR_REMOVE", "--exclude-operators=EXPR_REMOVE", dir}, "no operator enabled"},
		{"unknown format", []string{"run", "--format=xml", dir}, `unknown format "xml"`},
		{"sarif is not a list format", []string{"list", "--format=sarif", dir}, `unknown format "sarif"`},
		{"threshold out of range", []string{"run", "--threshold=101", dir}, "--threshold must be between 0 and 100"},
		{"parallel", []string{"run", "--parallel=0", dir}, "--parallel must be at least 1"},
		{"missing config", []string{"run", "--config=does-not-exist.yaml", dir}, "does-not-exist.yaml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, c.args...)
			if code != exitUsage {
				t.Errorf("exit %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr, c.want) {
				t.Errorf("stderr %q does not contain %q", stderr, c.want)
			}
		})
	}
}

func TestConfigErrors(t *testing.T) {
	cases := map[string]string{
		"version: 2\n":                                       "version must be 1",
		"version: 1\nthreshhold: 80\n":                       "field threshhold not found",
		"version: 1\nthreshold: 120\n":                       "threshold must be between 0 and 100",
		"version: 1\noperators:\n  exclude: [X]\n":           `unknown operator "X"`,
		"version: 1\nignore:\n  - line: 3\n":                 "needs a file and a line",
		"version: 1\nignore:\n  - pattern: \"(\"\n":          "invalid pattern",
		"version: 1\nignore:\n  - pattern: x\n    line: 3\n": "both a pattern and a file or line",
	}
	for content, want := range cases {
		path := filepath.Join(t.TempDir(), ".regodrill.yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCLI(t, "run", "--config="+path, "../../testdata/scenarios/expression-removal")
		if code != exitUsage || !strings.Contains(stderr, want) {
			t.Errorf("config %q: exit %d, stderr %q; want exit 1 and %q", content, code, stderr, want)
		}
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"policies/vendor/**", "policies/vendor/lib.rego", true},
		{"policies/vendor/**", "policies/vendor/a/b.rego", true},
		{"policies/vendor/**", "policies/authz.rego", false},
		{"**/generated_*.rego", "generated_x.rego", true},
		{"**/generated_*.rego", "a/b/generated_x.rego", true},
		{"*.rego", "a/b.rego", false},
		{"a/?.rego", "a/b.rego", true},
		{"a.b/*.rego", "aXb/c.rego", false},
	}
	for _, c := range cases {
		re, err := globRegexp(c.glob)
		if err != nil {
			t.Fatal(err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("%s matches %s: got %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

func TestVersionAndOperators(t *testing.T) {
	code, out, _ := runCLI(t, "version")
	if code != exitOK || !strings.HasPrefix(out, "regodrill ") || !strings.Contains(out, "OPA ") {
		t.Errorf("version: exit %d, %q", code, out)
	}
	code, out, _ = runCLI(t, "operators", "--format=json")
	var ops []struct{ ID string }
	if code != exitOK || json.Unmarshal([]byte(out), &ops) != nil || len(ops) != 10 {
		t.Errorf("operators: exit %d, %d operators, want 10", code, len(ops))
	}
}

func TestList(t *testing.T) {
	code, out, _ := runCLI(t, "list", "--operators=EXPR_REMOVE", "--format=json", "../../testdata/scenarios/expression-removal")
	var listing struct {
		Mutants []struct {
			Operator string
			Line     int
		}
	}
	if code != exitOK || json.Unmarshal([]byte(out), &listing) != nil {
		t.Fatalf("exit %d, %s", code, out)
	}
	if len(listing.Mutants) != 3 {
		t.Errorf("got %d mutants, want 3 (one per body expression)", len(listing.Mutants))
	}
	code, out, _ = runCLI(t, "list", "../../testdata/scenarios/inline-ignores")
	if code != exitOK || !strings.Contains(out, "(ignored: the gateway checks the level)") {
		t.Errorf("text listing does not show ignored mutants: exit %d\n%s", code, out)
	}
}

func TestSARIF(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.sarif")
	code, stdout, stderr := runCLI(t, "run", "--format=sarif", "--output="+out, "--parallel=2", "../../testdata/scenarios/expression-removal")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty with --output, got %q", stdout)
	}
	if !strings.Contains(stderr, "Mutation score:") {
		t.Errorf("the text summary should go to stderr with --output, got %q", stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string
					Rules []struct{ ID string }
				}
			}
			Results []struct {
				RuleID              string
				Level               string
				Message             struct{ Text string }
				PartialFingerprints map[string]string
				Locations           []struct {
					PhysicalLocation struct {
						ArtifactLocation struct{ URI string }
						Region           struct{ StartLine, StartColumn, EndLine, EndColumn int }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || log.Runs[0].Tool.Driver.Name != "regodrill" {
		t.Fatalf("not a regodrill SARIF 2.1.0 log: %s", data)
	}
	if len(log.Runs[0].Tool.Driver.Rules) != 10 {
		t.Errorf("got %d rules, want one per operator", len(log.Runs[0].Tool.Driver.Rules))
	}
	// expression-removal: with all operators, survivors exist; every result is a survivor or uncovered.
	results := log.Runs[0].Results
	if len(results) == 0 {
		t.Fatal("no results")
	}
	for _, r := range results {
		if r.Level != "warning" && r.Level != "note" {
			t.Errorf("level %q", r.Level)
		}
		if r.PartialFingerprints["regodrillId"] == "" || r.Message.Text == "" || len(r.Locations) != 1 {
			t.Errorf("incomplete result %+v", r)
		}
		loc := r.Locations[0].PhysicalLocation
		if strings.Contains(loc.ArtifactLocation.URI, `\`) || loc.Region.StartLine < 1 || loc.Region.StartColumn < 1 ||
			loc.Region.EndLine < loc.Region.StartLine || (loc.Region.EndLine == loc.Region.StartLine && loc.Region.EndColumn <= loc.Region.StartColumn) {
			t.Errorf("bad location %+v", loc)
		}
	}
}

// TestSince builds a git repository: a committed policy, then a change on one line and a new file.
// Only the mutants of the changed line and of the new file run.
func TestSince(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("policy.rego", "package p\n\nallow if {\n\tinput.a >= 1\n\tinput.b >= 2\n}\n")
	write("policy_test.rego", "package p_test\n\nimport data.p\n\ntest_allow if p.allow with input as {\"a\": 5, \"b\": 5, \"c\": 5}\n")
	gitRun("init", "-q", "-b", "main")
	gitRun("add", ".")
	gitRun("commit", "-q", "-m", "init")
	// Change line 5 (b) and add a new, untracked file.
	write("policy.rego", "package p\n\nallow if {\n\tinput.a >= 1\n\tinput.b >= 3\n}\n")
	write("extra.rego", "package p\n\nextra if input.c > 1\n")
	t.Chdir(dir)

	code, out, stderr := runCLI(t, "run", "--operators=COMP_BOUNDARY", "--format=json", "--since=main", ".")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var rep struct {
		Summary struct{ Since string }
		Mutants []struct {
			File string
			Line int
		}
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range rep.Mutants {
		got = append(got, fmt.Sprintf("%s:%d", filepath.Base(m.File), m.Line))
	}
	want := []string{"extra.rego:3", "policy.rego:5"}
	if strings.Join(got, ",") != strings.Join(want, ",") || rep.Summary.Since != "main" {
		t.Errorf("mutants %v, since %q; want %v and main", got, rep.Summary.Since, want)
	}

	code, _, stderr = runCLI(t, "run", "--since=no-such-ref", ".")
	if code != exitUsage || !strings.Contains(stderr, "no-such-ref") {
		t.Errorf("unknown ref: exit %d, %q", code, stderr)
	}
}

func TestTypeGuardHint(t *testing.T) {
	code, out, _ := runCLI(t, "run", "--format=json", "--operators=EXPR_REMOVE", "../../testdata/scenarios/builtin-errors")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var rep struct {
		Mutants []struct{ Original, Status, Message string }
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	hinted := 0
	for _, m := range rep.Mutants {
		has := strings.Contains(m.Message, "regodrill:ignore")
		if want := m.Status == "survived" && strings.HasPrefix(m.Original, "is_string"); has != want {
			t.Errorf("%q (%s): hint %v, want %v", m.Original, m.Status, has, want)
		}
		if has {
			hinted++
		}
	}
	if hinted != 1 {
		t.Errorf("%d hints, want 1 (the surviving guard of long_name)", hinted)
	}
}
