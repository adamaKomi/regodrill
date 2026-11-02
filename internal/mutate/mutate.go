// Package mutate generates mutants: one small text change applied to a policy file.
package mutate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/adamaKomi/regodrill/internal/project"
)

// Operator IDs, as in docs/mutation-testing-spec.md §5.
const (
	CompBoundary = "COMP_BOUNDARY"
	ExprRemove   = "EXPR_REMOVE"
	DefaultFlip  = "DEFAULT_FLIP"
)

// Operators lists the operators implemented by this build.
var Operators = []string{CompBoundary, ExprRemove, DefaultFlip}

// Mutant is one mutation of one file: the bytes [Start, End) of the source are replaced.
type Mutant struct {
	ID          string `json:"id"`
	Operator    string `json:"operator"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Start       int    `json:"-"`
	End         int    `json:"-"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}

// Apply returns the source of file with the mutation applied.
func (m *Mutant) Apply(src string) string {
	return src[:m.Start] + m.Replacement + src[m.End:]
}

// Generate returns the mutants of every non-test file, in a deterministic order.
func Generate(p *project.Project, enabled map[string]bool) []*Mutant {
	var out []*Mutant
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		g := &generator{file: f, enabled: enabled}
		g.compBoundary()
		g.exprRemove()
		g.defaultFlip()
		out = append(out, g.mutants...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Operator < b.Operator
	})
	seq := map[string]int{}
	for _, m := range out {
		key := fmt.Sprintf("%s:%d:%d:%s", m.File, m.Line, m.Column, m.Operator)
		m.ID = fmt.Sprintf("%s:%d", key, seq[key])
		seq[key]++
	}
	return out
}

type generator struct {
	file    *project.File
	enabled map[string]bool
	mutants []*Mutant
}

func (g *generator) add(op string, start, end int, replacement string) {
	if !g.enabled[op] || start < 0 || end > len(g.file.Source) || start >= end {
		return
	}
	line, col := position(g.file.Source, start)
	g.mutants = append(g.mutants, &Mutant{
		Operator: op, File: g.file.Path, Line: line, Column: col,
		Start: start, End: end, Original: g.file.Source[start:end], Replacement: replacement,
	})
}

// boundaries maps a comparison built-in to its symbol and to the symbol of its boundary mutant.
var boundaries = map[string][2]string{
	"gte": {">=", ">"},
	"gt":  {">", ">="},
	"lte": {"<=", "<"},
	"lt":  {"<", "<="},
}

// compBoundary: >= ↔ >, <= ↔ <, for infix comparisons written as expressions or nested terms.
func (g *generator) compBoundary() {
	visit := func(op ast.Ref, left, right *ast.Term) {
		syms, ok := boundaries[op.String()]
		if !ok || left == nil || right == nil || left.Location == nil || right.Location == nil {
			return
		}
		from := left.Location.Offset + len(left.Location.Text)
		to := right.Location.Offset
		if from > to || to > len(g.file.Source) {
			return
		}
		// The operator is the only token between the two operands of an infix call.
		between := g.file.Source[from:to]
		i := strings.Index(between, syms[0])
		if i < 0 || strings.TrimSpace(between) != syms[0] {
			return
		}
		g.add(CompBoundary, from+i, from+i+len(syms[0]), syms[1])
	}
	ast.WalkExprs(g.file.Module, func(e *ast.Expr) bool {
		if e.IsCall() && len(e.Operands()) == 2 {
			visit(e.Operator(), e.Operand(0), e.Operand(1))
		}
		return false
	})
	ast.WalkTerms(g.file.Module, func(t *ast.Term) bool {
		if c, ok := t.Value.(ast.Call); ok && len(c) == 3 {
			if r, ok := c[0].Value.(ast.Ref); ok {
				visit(r, c[1], c[2])
			}
		}
		return false
	})
}

// exprRemove: delete one expression of a rule body that has at least two.
func (g *generator) exprRemove() {
	for _, rule := range g.file.Module.Rules {
		for r := rule; r != nil; r = r.Else {
			if len(r.Body) < 2 {
				continue
			}
			for _, e := range r.Body {
				if e.Location == nil || e.Location.File != g.file.Path {
					continue
				}
				start := e.Location.Offset
				end := start + len(e.Location.Text)
				// Also drop a ';' separator that follows, so `a; b` becomes `b`.
				rest := g.file.Source[end:]
				if trimmed := strings.TrimLeft(rest, " \t"); strings.HasPrefix(trimmed, ";") {
					end += len(rest) - len(trimmed) + 1
				}
				g.add(ExprRemove, start, end, "")
			}
		}
	}
}

// defaultFlip: default x := true ↔ false.
func (g *generator) defaultFlip() {
	for _, rule := range g.file.Module.Rules {
		if !rule.Default || rule.Head.Value == nil || rule.Head.Value.Location == nil {
			continue
		}
		b, ok := rule.Head.Value.Value.(ast.Boolean)
		if !ok {
			continue
		}
		loc := rule.Head.Value.Location
		g.add(DefaultFlip, loc.Offset, loc.Offset+len(loc.Text), fmt.Sprint(!bool(b)))
	}
}

// position converts a byte offset to a 1-based line and column.
func position(src string, offset int) (int, int) {
	line := 1 + strings.Count(src[:offset], "\n")
	col := offset - strings.LastIndex(src[:offset], "\n")
	return line, col
}
