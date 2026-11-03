package mutate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/adamaKomi/regodrill/internal/project"
)

// Ignore marks the mutants of one line as checked by a person: they are reported as ignored and left out
// of the score (docs/mutation-testing-spec.md §7).
type Ignore struct {
	File string // path of the policy file, as loaded
	Line int
	// Pattern, when set, replaces File and Line: the ignore applies to every line of a policy file whose
	// text matches it (`input\.review\.kind\.kind ==` for the kind filters that Gatekeeper's match already
	// enforces).
	Pattern *regexp.Regexp
	// Operators narrows the ignore to these operator IDs; empty means every operator.
	Operators []string
	Reason    string
	// Origin says where the ignore comes from, for warnings: "policy.rego:13" or ".regodrill.yaml".
	Origin string
	used   bool
}

func (ig *Ignore) matches(m *Mutant, line string) bool {
	if ig.Pattern != nil {
		if !ig.Pattern.MatchString(line) {
			return false
		}
	} else if m.Line != ig.Line || !samePath(m.File, ig.File) {
		return false
	}
	if len(ig.Operators) == 0 {
		return true
	}
	for _, op := range ig.Operators {
		if op == m.Operator {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && aa == bb
}

const (
	annotation         = "regodrill:ignore"
	annotationNextLine = "regodrill:ignore-next-line"
)

// InlineIgnores reads the `# regodrill:ignore [IDS] [-- reason]` and `# regodrill:ignore-next-line ...`
// comments of the policy files. A comment that is not an annotation is skipped; an annotation with an
// unknown operator ID is a warning.
func InlineIgnores(p *project.Project) (ignores []*Ignore, warnings []string) {
	known := map[string]bool{}
	for _, op := range Operators {
		known[op] = true
	}
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, c := range f.Module.Comments {
			text := strings.TrimSpace(string(c.Text))
			line := c.Location.Row
			var rest string
			switch {
			case strings.HasPrefix(text, annotationNextLine):
				rest = text[len(annotationNextLine):]
				line++
			case strings.HasPrefix(text, annotation):
				rest = text[len(annotation):]
			default:
				continue
			}
			if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
				continue // regodrill:ignored, regodrill:ignore-foo: not an annotation
			}
			ig := &Ignore{File: f.Path, Line: line, Origin: fmt.Sprintf("%s:%d", f.Path, c.Location.Row)}
			ids, reason, _ := strings.Cut(rest, "--")
			ig.Reason = strings.TrimSpace(reason)
			for _, id := range strings.FieldsFunc(ids, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
				if !known[id] {
					warnings = append(warnings, fmt.Sprintf("%s: unknown operator %q in %s", ig.Origin, id, annotation))
					continue
				}
				ig.Operators = append(ig.Operators, id)
			}
			ignores = append(ignores, ig)
		}
	}
	return ignores, warnings
}

// ApplyIgnores splits mutants into the ones to run and the ignored ones, with the reason of each ignored
// mutant. An ignore that matches no mutant gets a warning, so stale annotations don't pile up, unless all
// its operators are disabled in this run (then it could not match).
func ApplyIgnores(p *project.Project, mutants []*Mutant, ignores []*Ignore, enabled map[string]bool) (run, ignored []*Mutant, reasons map[*Mutant]string, warnings []string) {
	reasons = map[*Mutant]string{}
	lines := map[string][]string{}
	for _, f := range p.Files {
		lines[f.Path] = strings.Split(f.Source, "\n")
	}
	for _, m := range mutants {
		matched := false
		text := ""
		if ls := lines[m.File]; m.Line-1 < len(ls) {
			text = strings.TrimRight(ls[m.Line-1], "\r")
		}
		for _, ig := range ignores {
			if ig.matches(m, text) {
				ig.used = true
				if !matched {
					reasons[m] = ig.Reason
				}
				matched = true
			}
		}
		if matched {
			ignored = append(ignored, m)
		} else {
			run = append(run, m)
		}
	}
	for _, ig := range ignores {
		active := len(ig.Operators) == 0
		for _, op := range ig.Operators {
			active = active || enabled[op]
		}
		if !ig.used && active {
			ops := "any operator"
			if len(ig.Operators) > 0 {
				ops = strings.Join(ig.Operators, ", ")
			}
			where := fmt.Sprintf("%s:%d", ig.File, ig.Line)
			if ig.Pattern != nil {
				where = "lines matching `" + ig.Pattern.String() + "`"
			}
			warnings = append(warnings, fmt.Sprintf("%s: ignore matches no mutant on %s (%s)", ig.Origin, where, ops))
		}
	}
	sort.Strings(warnings)
	return run, ignored, reasons, warnings
}
