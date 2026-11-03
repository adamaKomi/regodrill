package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	opaversion "github.com/open-policy-agent/opa/v1/version"

	"github.com/adamaKomi/regodrill/internal/engine"
	"github.com/adamaKomi/regodrill/internal/mutate"
)

// version of regodrill. Release builds set it with -ldflags "-X main.version=..." (goreleaser); `go install
// module@version` doesn't, but Go embeds the module version in the binary, so it is read from there.
var version = ""

func init() {
	if version == "" {
		version = moduleVersion()
	}
}

// moduleVersion is the main module's version from the build info ("v0.1.0" gives "0.1.0"), or "dev" for a
// build without one (go run, or a checkout without a version tag).
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

type summary struct {
	Score      *float64 `json:"score"`
	Total      int      `json:"total"`
	Killed     int      `json:"killed"`
	Timeout    int      `json:"timeout"`
	Survived   int      `json:"survived"`
	NoCoverage int      `json:"noCoverage"`
	Stillborn  int      `json:"stillborn"`
	Equivalent int      `json:"equivalent"`
	Ignored    int      `json:"ignored"`
	PatchOK    int      `json:"patchOk"`
	Tests      int      `json:"tests"`
	Parallel   int      `json:"parallel"`
	Select     bool     `json:"select"`
	Threshold  *float64 `json:"threshold,omitempty"`
	Since      string   `json:"since,omitempty"`
	BaselineMs float64  `json:"baselineMs"`
	CoverageMs float64  `json:"coverageMs"`
	MutationMs float64  `json:"mutationMs"`
	DurationMs float64  `json:"durationMs"`
}

type tool struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	OPAVersion string `json:"opaVersion"`
}

type report struct {
	SchemaVersion int               `json:"schemaVersion"`
	Tool          tool              `json:"tool"`
	Summary       summary           `json:"summary"`
	Warnings      []string          `json:"warnings,omitempty"`
	Mutants       []*engine.Result  `json:"mutants"`
	Operators     map[string]opStat `json:"operators"`
}

// opStat counts the results of one operator. Timeouts count as killed.
type opStat struct {
	Total      int `json:"total"`
	Killed     int `json:"killed"`
	Survived   int `json:"survived"`
	NoCoverage int `json:"noCoverage"`
	Stillborn  int `json:"stillborn"`
	Equivalent int `json:"equivalent"`
	Ignored    int `json:"ignored"`
}

// timing of a run, for the summary.
type timing struct {
	start, coverageDone, end time.Time
	baseline                 time.Duration
	tests                    int
}

func newReport(results []*engine.Result, t timing, parallel int, sel bool) *report {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	s := summary{
		Total: len(results), Tests: t.tests, Parallel: parallel, Select: sel,
		BaselineMs: ms(t.baseline), CoverageMs: ms(t.coverageDone.Sub(t.start)),
		MutationMs: ms(t.end.Sub(t.coverageDone)), DurationMs: ms(t.end.Sub(t.start)),
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
		case engine.Ignored:
			s.Ignored++
			st.Ignored++
		case engine.Equivalent:
			s.Equivalent++
			st.Equivalent++
		}
		if r.PatchOK {
			s.PatchOK++
		}
		ops[r.Operator] = st
		r.Message = message(r)
	}
	if scorable := s.Killed + s.Timeout + s.Survived + s.NoCoverage; scorable > 0 {
		score := float64(s.Killed+s.Timeout) / float64(scorable) * 100
		s.Score = &score
	}
	return &report{
		SchemaVersion: 1,
		Tool:          tool{Name: "regodrill", Version: version, OPAVersion: opaversion.Version},
		Summary:       s, Mutants: results, Operators: ops,
	}
}

// message explains a result in one sentence.
func message(r *engine.Result) string {
	change := r.Change()
	switch r.Status {
	case engine.Killed:
		return fmt.Sprintf("%s makes %s fail.", change, r.KilledBy)
	case engine.Timeout:
		return fmt.Sprintf("%s makes the tests time out (counted as caught).", change)
	case engine.Survived:
		msg := fmt.Sprintf("%s does not make any test fail.", change)
		if isTypeGuardRemoval(r.Mutant) {
			// The survivor review found these to be the most common equivalent mutants (docs/spike-m0-survivor-review.md).
			msg += " If a later built-in fails on other types anyway, OPA makes the result undefined and the guard changes nothing: check it, then mark it with # regodrill:ignore."
		}
		return msg
	case engine.NoCoverage:
		return fmt.Sprintf("%s does not make any test fail: no test evaluates this line.", change)
	case engine.Stillborn:
		return fmt.Sprintf("%s does not compile: %s", change, r.Error)
	case engine.Equivalent:
		return fmt.Sprintf("%s does not change the policy: the result is the same rules as the original.", change)
	case engine.Ignored:
		if r.IgnoreReason != "" {
			return "Ignored: " + r.IgnoreReason
		}
		return "Ignored."
	}
	return ""
}

// typeGuards are the built-ins that test a value's type.
var typeGuards = []string{"is_string(", "is_number(", "is_array(", "is_object(", "is_set(", "is_boolean(", "is_null("}

// isTypeGuardRemoval reports whether a mutant removes a type check, with or without `not`.
func isTypeGuardRemoval(m *mutate.Mutant) bool {
	if m.Operator != mutate.ExprRemove {
		return false
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.Original), "not "))
	for _, g := range typeGuards {
		if strings.HasPrefix(text, g) {
			return true
		}
	}
	return false
}

func writeJSON(w io.Writer, rep *report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// isTerminal reports whether w is a terminal (a character device).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// useColor follows the NO_COLOR convention (https://no-color.org): colors only on a terminal, and never when
// NO_COLOR is set to a non-empty value.
func useColor(w io.Writer) bool { return isTerminal(w) && os.Getenv("NO_COLOR") == "" }

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiRed    = "\033[31m"
)

func paint(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + ansiReset
}

func writeText(w io.Writer, rep *report, color bool) {
	for _, r := range rep.Mutants {
		if r.Status != engine.Survived && r.Status != engine.NoCoverage {
			continue
		}
		change := fmt.Sprintf("%q → %q", oneLine(r.Original), r.Replacement)
		status := strings.ToUpper(strings.ReplaceAll(string(r.Status), "_", " "))
		code := ansiYellow
		if r.Status == engine.NoCoverage {
			code = ansiCyan
		}
		fmt.Fprintf(w, "%s:%d:%d  %-15s %-60s %s\n", r.File, r.Line, r.Column, r.Operator, change, paint(color, code, status))
	}
	writeSummary(w, rep, color)
}

func writeSummary(w io.Writer, rep *report, color bool) {
	s := rep.Summary
	score := "n/a"
	if s.Score != nil {
		score = fmt.Sprintf("%.1f %%", *s.Score)
		code := ansiGreen
		if s.Threshold != nil && *s.Score < *s.Threshold {
			code = ansiRed
		}
		score = paint(color, ansiBold+code, score)
	}
	fmt.Fprintf(w, "\nMutation score: %s   killed %d · timeout %d · survived %d · no coverage %d · stillborn %d · equivalent %d · ignored %d\n",
		score, s.Killed, s.Timeout, s.Survived, s.NoCoverage, s.Stillborn, s.Equivalent, s.Ignored)
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

// SARIF 2.1.0, as GitHub code scanning reads it (docs/mutation-testing-spec.md §9.3).
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
	FullDescription  sarifText `json:"fullDescription"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region sarifRegion `json:"region"`
	} `json:"physicalLocation"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine     int `json:"endLine"`
	EndColumn   int `json:"endColumn"`
}

// writeSARIF reports survived mutants as warnings and uncovered ones as notes. URIs are relative to the
// current directory, with `/`, which is how code scanning matches them with the repository.
func writeSARIF(w io.Writer, rep *report) error {
	driver := sarifDriver{Name: "regodrill", Version: version, InformationURI: "https://github.com/adamaKomi/regodrill"}
	for _, d := range mutate.Descriptions {
		driver.Rules = append(driver.Rules, sarifRule{
			ID:               d.ID,
			ShortDescription: sarifText{d.Summary},
			FullDescription:  sarifText{fmt.Sprintf("%s. Example: %s. Risk: %s.", d.Summary, d.Example, d.Risk)},
		})
	}
	run := sarifRun{Tool: sarifTool{Driver: driver}, Results: []sarifResult{}}
	cwd, _ := os.Getwd()
	for _, r := range rep.Mutants {
		level := ""
		switch r.Status {
		case engine.Survived:
			level = "warning"
		case engine.NoCoverage:
			level = "note"
		default:
			continue
		}
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI = sarifURI(cwd, r.File)
		end := r.Range.End
		if end == r.Range.Start {
			end.Column++ // an insertion: point at the character after it, so the region is not empty
		}
		loc.PhysicalLocation.Region = sarifRegion{r.Range.Start.Line, r.Range.Start.Column, end.Line, end.Column}
		run.Results = append(run.Results, sarifResult{
			RuleID: r.Operator, Level: level, Message: sarifText{r.Message},
			Locations:           []sarifLocation{loc},
			PartialFingerprints: map[string]string{"regodrillId": r.ID},
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{Schema: "https://json.schemastore.org/sarif-2.1.0.json", Version: "2.1.0", Runs: []sarifRun{run}})
}

func sarifURI(cwd, file string) string {
	if filepath.IsAbs(file) && cwd != "" {
		if rel, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(rel, "..") {
			file = rel
		}
	}
	return filepath.ToSlash(filepath.Clean(file))
}
