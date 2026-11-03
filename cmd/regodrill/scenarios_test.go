package main

// Scenario catalog: every directory under testdata/scenarios is one end-to-end run of the CLI, compared with
// the expected.txt file next to it. See testdata/scenarios/README.md.
//
// Regenerate the expected files with `go test ./cmd/regodrill -run TestScenarios -update`, then review every
// changed line: an expected file is a claim about what the tool must do, not a snapshot of what it does.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite expected.txt of each scenario")

const scenarioRoot = "../../testdata/scenarios"

// windowsPath matches a relative Windows file path such as lib\text.rego.
var windowsPath = regexp.MustCompile(`[\w.\-]+(?:\\[\w.\-]+)*\\[\w.\-]+\.(?:rego|yaml|yml|json)\b`)

func TestScenarios(t *testing.T) {
	entries, err := os.ReadDir(scenarioRoot)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n++
		dir := filepath.Join(scenarioRoot, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			t.Parallel()
			args, paths := scenarioArgs(t, dir)
			// The verdicts must not depend on test selection: run both ways.
			selected := runScenario(t, dir, args, paths)
			all := runScenario(t, dir, append(args, "--no-select"), paths)
			if selected != all {
				t.Fatalf("verdicts differ with and without test selection\n--- with selection\n%s--- without\n%s", selected, all)
			}
			expectedFile := filepath.Join(dir, "expected.txt")
			if *update {
				if err := os.WriteFile(expectedFile, []byte(selected), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(expectedFile)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if selected != string(want) {
				t.Errorf("unexpected result\n--- got\n%s--- want\n%s", selected, want)
			}
		})
	}
	if n == 0 {
		t.Fatal("no scenario found")
	}
}

// scenarioArgs reads the optional `args` file: one flag (`--name=value` form) or path per line, `#` starts a
// comment. Paths are relative to the scenario directory; `abs:` before a path makes it absolute. In a flag,
// `{dir}` is the scenario directory (`--config={dir}/regodrill.yaml`). Without any path, the directory
// itself is the path, unless a line says `(no paths)`.
func scenarioArgs(t *testing.T, dir string) (flags, paths []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "args"))
	if os.IsNotExist(err) {
		return nil, []string{dir}
	}
	if err != nil {
		t.Fatal(err)
	}
	noPaths := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case line == "(no paths)":
			noPaths = true // let the configuration file give the paths
		case strings.HasPrefix(line, "-"):
			flags = append(flags, strings.ReplaceAll(line, "{dir}", dir))
		case strings.HasPrefix(line, "abs:"):
			abs, err := filepath.Abs(filepath.Join(dir, strings.TrimPrefix(line, "abs:")))
			if err != nil {
				t.Fatal(err)
			}
			paths = append(paths, abs)
		default:
			paths = append(paths, filepath.Join(dir, line))
		}
	}
	if len(paths) == 0 && !noPaths {
		paths = []string{dir}
	}
	return flags, paths
}

// runScenario runs the CLI and renders what a scenario asserts: the exit code, the error message if any, and
// one line per mutant. Timings and summary counts are left out (the counts follow from the mutant lines).
func runScenario(t *testing.T, dir string, flags, paths []string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	args := append([]string{"run", "--format=json", "--parallel=2"}, flags...)
	code := run(append(args, paths...), &stdout, &stderr)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Expected files use `/` in file paths, whatever the platform. Only path-like tokens are converted: a
	// backslash elsewhere (a regular expression in a warning) is text.
	clean := func(s string) string {
		for _, prefix := range []string{absDir, dir} {
			s = strings.ReplaceAll(s, prefix+string(filepath.Separator), "")
			s = strings.ReplaceAll(s, prefix+"/", "")
		}
		if filepath.Separator == '\\' {
			s = windowsPath.ReplaceAllStringFunc(s, filepath.ToSlash)
		}
		return s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exit %d\n", code)
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		for _, line := range strings.Split(clean(msg), "\n") {
			fmt.Fprintf(&b, "stderr: %s\n", line)
		}
	}
	if code != exitOK && code != exitThreshold {
		return b.String()
	}
	var rep struct {
		Mutants []struct {
			File, Operator, Original, Replacement, Status, KilledBy, Error, Rule, IgnoreReason string
			Line, Column                                                                       int
			PatchOK                                                                            bool
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, stdout.String())
	}
	for _, m := range rep.Mutants {
		// A mutant that compiles must change the policy: a patch that parses to the same AST would be
		// reported as a survivor that no test could ever kill. Ignored mutants are not run, so not checked.
		if m.Status != "stillborn" && m.Status != "ignored" && m.Status != "equivalent" && !m.PatchOK {
			t.Errorf("%s:%d:%d %s: the patch does not change the AST", m.File, m.Line, m.Column, m.Operator)
		}
		fmt.Fprintf(&b, "%s:%d:%d %s %s %q -> %q %s", clean(m.File), m.Line, m.Column, m.Operator, m.Rule, oneLine(m.Original), m.Replacement, m.Status)
		switch {
		case m.KilledBy != "":
			fmt.Fprintf(&b, " by %s", m.KilledBy)
		case m.Error != "":
			fmt.Fprintf(&b, ": %s", clean(m.Error))
		case m.IgnoreReason != "":
			fmt.Fprintf(&b, ": %s", m.IgnoreReason)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
