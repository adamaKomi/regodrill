package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The JSON report as documented in docs/report-format.md. Decoding with DisallowUnknownFields makes any
// undocumented field a test failure: update the document and this copy together.
type contractReport struct {
	SchemaVersion int `json:"schemaVersion"`
	Tool          struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		OPAVersion string `json:"opaVersion"`
	} `json:"tool"`
	Summary struct {
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
		Threshold  *float64 `json:"threshold"`
		Since      string   `json:"since"`
		BaselineMs float64  `json:"baselineMs"`
		CoverageMs float64  `json:"coverageMs"`
		MutationMs float64  `json:"mutationMs"`
		DurationMs float64  `json:"durationMs"`
	} `json:"summary"`
	Warnings  []string         `json:"warnings"`
	Mutants   []contractMutant `json:"mutants"`
	Operators map[string]struct {
		Total      int `json:"total"`
		Killed     int `json:"killed"`
		Survived   int `json:"survived"`
		NoCoverage int `json:"noCoverage"`
		Stillborn  int `json:"stillborn"`
		Equivalent int `json:"equivalent"`
		Ignored    int `json:"ignored"`
	} `json:"operators"`
}

type contractPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type contractMutant struct {
	ID       string `json:"id"`
	Operator string `json:"operator"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Range    struct {
		Start contractPosition `json:"start"`
		End   contractPosition `json:"end"`
	} `json:"range"`
	Original      string   `json:"original"`
	Replacement   string   `json:"replacement"`
	Rule          string   `json:"rule"`
	Status        string   `json:"status"`
	Message       string   `json:"message"`
	KilledBy      string   `json:"killedBy"`
	CoveringTests []string `json:"coveringTests"`
	SelectedTests []string `json:"selectedTests"`
	Error         string   `json:"error"`
	IgnoreReason  string   `json:"ignoreReason"`
	PatchOK       bool     `json:"patchOk"`
	DurationMs    float64  `json:"durationMs"`
}

func TestReportContract(t *testing.T) {
	dirs := []string{"inline-ignores", "stillborn-filter", "config-file", "operator-not-toggle", "comparison-boundaries", "equivalent-mutant"}
	statuses := map[string]bool{}
	for _, d := range dirs {
		dir := scenarioRoot + "/" + d
		flags, paths := scenarioArgs(t, dir)
		code, out, stderr := runCLI(t, append(append([]string{"run", "--format=json", "--parallel=2"}, flags...), paths...)...)
		if code != exitOK && code != exitThreshold {
			t.Fatalf("%s: exit %d: %s", d, code, stderr)
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(out)))
		dec.DisallowUnknownFields()
		var rep contractReport
		if err := dec.Decode(&rep); err != nil {
			t.Fatalf("%s: the report does not match docs/report-format.md: %v", d, err)
		}
		if rep.SchemaVersion != 1 || rep.Tool.Name != "regodrill" || rep.Tool.Version == "" || rep.Tool.OPAVersion == "" {
			t.Errorf("%s: header %+v", d, rep.Tool)
		}
		counts := map[string]int{}
		for _, m := range rep.Mutants {
			statuses[m.Status] = true
			counts[m.Status]++
			if !strings.HasPrefix(m.ID, m.File+":") || m.Message == "" || m.Rule == "" || !strings.HasPrefix(m.Rule, "data.") {
				t.Errorf("%s: incomplete mutant %+v", d, m)
			}
			if m.Range.Start != (contractPosition{m.Line, m.Column}) {
				t.Errorf("%s: %s: range starts at %v, not at line and column", d, m.ID, m.Range.Start)
			}
			if m.Range.End.Line < m.Range.Start.Line || (m.Range.End.Line == m.Range.Start.Line && m.Range.End.Column < m.Range.Start.Column) {
				t.Errorf("%s: %s: range ends before it starts", d, m.ID)
			}
			switch m.Status {
			case "killed":
				if m.KilledBy == "" {
					t.Errorf("%s: %s: killed without killedBy", d, m.ID)
				}
			case "stillborn":
				if m.Error == "" {
					t.Errorf("%s: %s: stillborn without error", d, m.ID)
				}
			case "equivalent":
				if m.PatchOK {
					t.Errorf("%s: %s: equivalent with patchOk", d, m.ID)
				}
			case "survived", "no_coverage", "timeout", "ignored":
			default:
				t.Errorf("%s: %s: unknown status %q", d, m.ID, m.Status)
			}
		}
		s := rep.Summary
		if s.Total != len(rep.Mutants) || s.Killed != counts["killed"] || s.Survived != counts["survived"] ||
			s.NoCoverage != counts["no_coverage"] || s.Stillborn != counts["stillborn"] || s.Ignored != counts["ignored"] ||
			s.Timeout != counts["timeout"] || s.Equivalent != counts["equivalent"] {
			t.Errorf("%s: summary %+v does not match the mutants %v", d, s, counts)
		}
	}
	for _, st := range []string{"killed", "survived", "no_coverage", "stillborn", "ignored", "equivalent"} {
		if !statuses[st] {
			t.Errorf("no scenario of this test produced a %s mutant: the contract is not exercised for it", st)
		}
	}
}
