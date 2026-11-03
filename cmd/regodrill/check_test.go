package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const checkFixtures = "../../testdata/check/"

type checkCase struct {
	name  string
	args  []string
	code  int
	codes []string // error codes, in order ("" for an error without code)
	rows  []int    // rows of the first errors, when they matter
}

var checkCases = []checkCase{
	{name: "valid", args: []string{checkFixtures + "valid"}},
	{name: "parse error", args: []string{checkFixtures + "parse-error"}, code: 1,
		codes: []string{"rego_parse_error"}, rows: []int{5}},
	{name: "unsafe variable", args: []string{checkFixtures + "unsafe-var"}, code: 1,
		codes: []string{"rego_unsafe_var_error"}, rows: []int{4}},
	{name: "v0 syntax read as v1", args: []string{checkFixtures + "v0"}, code: 1,
		codes: []string{"rego_parse_error", "rego_parse_error", "rego_parse_error"}, rows: []int{3, 7, 7}},
	{name: "v0 syntax with --v0-compatible", args: []string{"--v0-compatible", checkFixtures + "v0"}},
	{name: "type errors", args: []string{checkFixtures + "type-errors"}, code: 1,
		codes: []string{"rego_type_error", "rego_type_error"}, rows: []int{4, 8}},
	{name: "default error limit", args: []string{checkFixtures + "many-errors"}, code: 1,
		codes: append(repeat("rego_unsafe_var_error", 10), "rego_compile_error")},
	{name: "no error limit", args: []string{"--max-errors", "0", checkFixtures + "many-errors"}, code: 1,
		codes: repeat("rego_unsafe_var_error", 12)},
	{name: "short error limit flag", args: []string{"-m", "3", checkFixtures + "many-errors"}, code: 1,
		codes: append(repeat("rego_unsafe_var_error", 3), "rego_compile_error")},
	{name: "data files are not loaded", args: []string{checkFixtures + "data-ignored"}},
	{name: "missing path", args: []string{checkFixtures + "does-not-exist"}, code: 1, codes: []string{""}},
	{name: "ignored directories", args: []string{"--ignore", "type-errors", "--ignore", "many-errors",
		"--ignore", "parse-error", "--ignore", "unsafe-var", "--ignore", "v0", checkFixtures}},
	{name: "files from two directories", args: []string{checkFixtures + "unsafe-var/policy.rego",
		checkFixtures + "type-errors/policy.rego"}, code: 1, codes: []string{"rego_unsafe_var_error"}},
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestCheck(t *testing.T) {
	for _, c := range checkCases {
		t.Run(c.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, append([]string{"check", "--format", "json"}, c.args...)...)
			if code != c.code {
				t.Fatalf("exit code %d, want %d; stderr:\n%s", code, c.code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got:\n%s", stdout)
			}
			if c.code == 0 {
				if stderr != "" {
					t.Errorf("stderr should be empty, got:\n%s", stderr)
				}
				return
			}
			var out struct {
				Errors []struct {
					Code     string
					Message  string
					Location *struct{ Row int }
					Details  any
				}
			}
			if err := json.Unmarshal([]byte(stderr), &out); err != nil {
				t.Fatalf("stderr is not JSON: %v\n%s", err, stderr)
			}
			var codes []string
			for _, e := range out.Errors {
				codes = append(codes, e.Code)
				if e.Message == "" {
					t.Errorf("error without message: %+v", e)
				}
			}
			if strings.Join(codes, ",") != strings.Join(c.codes, ",") {
				t.Errorf("codes %q, want %q", codes, c.codes)
			}
			for i, row := range c.rows {
				if i >= len(out.Errors) || out.Errors[i].Location == nil || out.Errors[i].Location.Row != row {
					t.Errorf("error %d: want row %d, got %+v", i, row, out.Errors[i:])
				}
			}
			if c.name == "type errors" && out.Errors[0].Details == nil {
				t.Error("a type error keeps its details (have, want)")
			}
		})
	}
}

func TestCheckPretty(t *testing.T) {
	code, stdout, stderr := runCLI(t, "check", checkFixtures+"v0")
	if code != 1 || stdout != "" {
		t.Fatalf("exit code %d, stdout %q", code, stdout)
	}
	if !strings.HasPrefix(stderr, "3 errors occurred during loading:\n") ||
		!strings.Contains(stderr, "policy.rego:3: rego_parse_error: `if` keyword is required before rule body") {
		t.Errorf("unexpected text output:\n%s", stderr)
	}
}

func TestCheckUsage(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no path", []string{"check"}, "specify at least one file"},
		{"unknown format", []string{"check", "--format", "xml", checkFixtures + "valid"}, `unknown format "xml"`},
		{"unknown flag", []string{"check", "--strict", checkFixtures + "valid"}, "flag provided but not defined: -strict"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, c.args...)
			if code != exitUsage || !strings.Contains(stderr, c.want) {
				t.Errorf("exit code %d, stderr %q; want %d and %q", code, stderr, exitUsage, c.want)
			}
		})
	}
}

// TestCheckMatchesOPA compares the whole output of `regodrill check` with `opa check` (same OPA version as
// the library: see go.mod), byte for byte. It needs an opa binary on the PATH; REQUIRE_OPA=1 makes it
// mandatory (CI on Linux).
func TestCheckMatchesOPA(t *testing.T) {
	opa, err := exec.LookPath("opa")
	if err != nil {
		if os.Getenv("REQUIRE_OPA") == "1" {
			t.Fatal("REQUIRE_OPA=1 but opa is not on the PATH")
		}
		t.Skip("no opa binary on the PATH")
	}
	for _, c := range checkCases {
		for _, format := range []string{"json", "pretty"} {
			args := append([]string{"check", "--format", format}, c.args...)
			t.Run(c.name+"/"+format, func(t *testing.T) {
				var out, errb bytes.Buffer
				cmd := exec.Command(opa, args...)
				cmd.Stdout, cmd.Stderr = &out, &errb
				opaCode := 0
				if err := cmd.Run(); err != nil {
					var exit *exec.ExitError
					if !errors.As(err, &exit) {
						t.Fatal(err)
					}
					opaCode = exit.ExitCode()
				}
				code, stdout, stderr := runCLI(t, args...)
				if code != opaCode || stdout != out.String() || stderr != errb.String() {
					t.Errorf("regodrill (exit %d):\n%s%s\nopa (exit %d):\n%s%s", code, stdout, stderr, opaCode, out.String(), errb.String())
				}
			})
		}
	}
}
