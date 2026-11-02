// Command regodrill runs mutation testing on Rego policies.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/adamaKomi/regodrill/internal/engine"
	"github.com/adamaKomi/regodrill/internal/mutate"
	"github.com/adamaKomi/regodrill/internal/project"
)

// Exit codes, as in docs/mutation-testing-spec.md §4.
const (
	exitOK       = 0
	exitUsage    = 1
	exitBaseline = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, "usage: regodrill run [flags] [paths...]")
		return exitUsage
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "report format: text or json")
	parallel := fs.Int("parallel", runtime.NumCPU(), "mutants evaluated in parallel")
	noSelect := fs.Bool("no-select", false, "run every test for every mutant instead of the covering tests only")
	timeoutFactor := fs.Float64("timeout-factor", 5, "a mutant times out after this many times the baseline duration of its tests")
	v0 := fs.Bool("v0-compatible", false, "parse policies as Rego v0")
	operators := fs.String("operators", strings.Join(mutate.Operators, ","), "comma-separated operators to enable")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	enabled := map[string]bool{}
	for _, op := range strings.Split(*operators, ",") {
		enabled[strings.TrimSpace(op)] = true
	}

	version := ast.RegoV1
	if *v0 {
		version = ast.RegoV0
	}
	p, err := project.Load(paths, version)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		// Files that don't parse or load mean the baseline cannot compile; a missing path is a usage error.
		var invalid *project.InvalidError
		if errors.As(err, &invalid) {
			return exitBaseline
		}
		return exitUsage
	}

	ctx := context.Background()
	start := time.Now()
	baseline, err := engine.RunBaseline(ctx, p)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitBaseline
	}
	coverageDone := time.Now()

	mutants := mutate.Generate(p, enabled)
	results := engine.Run(ctx, p, baseline, mutants, engine.Options{
		Parallel: *parallel, Select: !*noSelect, TimeoutFactor: *timeoutFactor,
	})
	rep := newReport(results, baseline, *parallel, !*noSelect, start, coverageDone, time.Now())

	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
		return exitOK
	}
	writeText(stdout, rep)
	return exitOK
}

type summary struct {
	Score      *float64 `json:"score"`
	Total      int      `json:"total"`
	Killed     int      `json:"killed"`
	Timeout    int      `json:"timeout"`
	Survived   int      `json:"survived"`
	NoCoverage int      `json:"noCoverage"`
	Stillborn  int      `json:"stillborn"`
	PatchOK    int      `json:"patchOk"`
	Tests      int      `json:"tests"`
	Parallel   int      `json:"parallel"`
	Select     bool     `json:"select"`
	BaselineMs float64  `json:"baselineMs"`
	CoverageMs float64  `json:"coverageMs"`
	MutationMs float64  `json:"mutationMs"`
	DurationMs float64  `json:"durationMs"`
}

type report struct {
	SchemaVersion int               `json:"schemaVersion"`
	Summary       summary           `json:"summary"`
	Mutants       []*engine.Result  `json:"mutants"`
	Operators     map[string]opStat `json:"operators"`
}

type opStat struct {
	Total, Killed, Survived, NoCoverage, Stillborn int
}

func newReport(results []*engine.Result, b *engine.Baseline, parallel int, sel bool, start, covDone, end time.Time) *report {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	s := summary{
		Total: len(results), Tests: len(b.Tests), Parallel: parallel, Select: sel,
		BaselineMs: ms(b.Duration), CoverageMs: ms(covDone.Sub(start)),
		MutationMs: ms(end.Sub(covDone)), DurationMs: ms(end.Sub(start)),
	}
	ops := map[string]opStat{}
	for _, r := range results {
		st := ops[r.Operator]
		st.Total++
		switch r.Status {
		case engine.Killed:
			s.Killed++
			st.Killed++
		case engine.Timeout:
			s.Timeout++
			st.Killed++
		case engine.Survived:
			s.Survived++
			st.Survived++
		case engine.NoCoverage:
			s.NoCoverage++
			st.NoCoverage++
		case engine.Stillborn:
			s.Stillborn++
			st.Stillborn++
		}
		if r.PatchOK {
			s.PatchOK++
		}
		ops[r.Operator] = st
	}
	if scorable := s.Killed + s.Timeout + s.Survived + s.NoCoverage; scorable > 0 {
		score := float64(s.Killed+s.Timeout) / float64(scorable) * 100
		s.Score = &score
	}
	return &report{SchemaVersion: 1, Summary: s, Mutants: results, Operators: ops}
}

func writeText(w io.Writer, rep *report) {
	for _, r := range rep.Mutants {
		if r.Status != engine.Survived && r.Status != engine.NoCoverage {
			continue
		}
		change := fmt.Sprintf("%q → %q", oneLine(r.Original), r.Replacement)
		fmt.Fprintf(w, "%s:%d:%d  %-14s %-60s %s\n", r.File, r.Line, r.Column, r.Operator, change, strings.ToUpper(strings.ReplaceAll(string(r.Status), "_", " ")))
	}
	s := rep.Summary
	score := "n/a"
	if s.Score != nil {
		score = fmt.Sprintf("%.1f %%", *s.Score)
	}
	fmt.Fprintf(w, "\nMutation score: %s   killed %d · timeout %d · survived %d · no coverage %d · stillborn %d\n",
		score, s.Killed, s.Timeout, s.Survived, s.NoCoverage, s.Stillborn)
	fmt.Fprintf(w, "Duration: %.0f ms (baseline + coverage %.0f ms, %d mutants, %d workers, test selection %v)\n",
		s.DurationMs, s.CoverageMs, s.Total, s.Parallel, s.Select)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 50 {
		s = s[:47] + "..."
	}
	return s
}
