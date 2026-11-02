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
	// Rule is the path of the enclosing rule (package and ground prefix of the head), e.g. data.pkg.allow.
	// Every definition of the same rule, including its default and else branches, shares this path.
	Rule    string  `json:"rule"`
	RuleRef ast.Ref `json:"-"`
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
	rule    ast.Ref // path of the rule being visited
	mutants []*Mutant
}

// eachRule calls fn for every rule of the file, with g.rule set to its path.
func (g *generator) eachRule(fn func(*ast.Rule)) {
	for _, rule := range g.file.Module.Rules {
		g.rule = rule.Path()
		fn(rule)
	}
	g.rule = nil
}

func (g *generator) add(op string, start, end int, replacement string) {
	if !g.enabled[op] || start < 0 || end > len(g.file.Source) || start >= end {
		return
	}
	line, col := position(g.file.Source, start)
	g.mutants = append(g.mutants, &Mutant{
		Operator: op, File: g.file.Path, Line: line, Column: col,
		Start: start, End: end, Original: g.file.Source[start:end], Replacement: replacement,
		Rule: g.rule.String(), RuleRef: g.rule,
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
	g.eachRule(func(rule *ast.Rule) {
		ast.WalkExprs(rule, func(e *ast.Expr) bool {
			if e.IsCall() && len(e.Operands()) == 2 {
				visit(e.Operator(), e.Operand(0), e.Operand(1))
			}
			return false
		})
		ast.WalkTerms(rule, func(t *ast.Term) bool {
			if c, ok := t.Value.(ast.Call); ok && len(c) == 3 {
				if r, ok := c[0].Value.(ast.Ref); ok {
					visit(r, c[1], c[2])
				}
			}
			return false
		})
	})
}

// exprRemove: delete one expression of a rule body that has at least two. An expression that declares a
// variable the rest of the rule needs is kept: removing it could only produce a stillborn mutant.
func (g *generator) exprRemove() {
	g.eachRule(func(rule *ast.Rule) {
		for r := rule; r != nil; r = r.Else {
			if len(r.Body) < 2 {
				continue
			}
			for _, e := range r.Body {
				if e.Location == nil || e.Location.File != g.file.Path || declaresUsedVar(r, e) {
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
	})
}

// declaresUsedVar reports whether removing e from the body of r leaves a variable unbound: e declares it
// (`x := ...`, `[a, b] := ...`, `some x in ...`), the head or another expression uses it, and no other
// expression could bind it instead. Such a mutant never compiles (rego_unsafe_var_error), so it is not
// generated. The check is conservative: when in doubt, the mutant is kept and the compiler decides.
func declaresUsedVar(r *ast.Rule, e *ast.Expr) bool {
	declared := declaredVars(e)
	if len(declared) == 0 {
		return false
	}
	used, bindable := ast.NewVarSet(), ast.NewVarSet()
	ast.WalkVars(r.Head, func(v ast.Var) bool {
		used.Add(v)
		return false
	})
	for _, other := range r.Body {
		if other == e {
			continue
		}
		ast.WalkVars(other, func(v ast.Var) bool {
			used.Add(v)
			return false
		})
		bindingVars(other, bindable)
	}
	for v := range declared {
		if used.Contains(v) && !bindable.Contains(v) {
			return true
		}
	}
	return false
}

// bindingVars adds to out the variables that e could bind on its own: operands of a unification (`x = y`),
// keys of a reference (`input.items[x]`, which iterates), and the last argument of a call that may be an
// output argument (`count(xs, n)`, or any user function with two arguments or more).
func bindingVars(e *ast.Expr, out ast.VarSet) {
	addVars := func(x any) {
		ast.WalkVars(x, func(v ast.Var) bool {
			out.Add(v)
			return false
		})
	}
	if e.IsEquality() {
		for _, t := range e.Operands() {
			addVars(t)
		}
	}
	ast.WalkRefs(e, func(ref ast.Ref) bool {
		for _, t := range ref[1:] {
			addVars(t)
		}
		return false
	})
	visitCall := func(op ast.Ref, args []*ast.Term) {
		if len(args) == 0 {
			return
		}
		if b, ok := ast.BuiltinMap[op.String()]; ok {
			if len(args) <= len(b.Decl.FuncArgs().Args) {
				return
			}
		} else if len(args) < 2 {
			return
		}
		addVars(args[len(args)-1])
	}
	if e.IsCall() {
		visitCall(e.Operator(), e.Operands())
	}
	ast.WalkTerms(e, func(t *ast.Term) bool {
		if c, ok := t.Value.(ast.Call); ok {
			if op, ok := c[0].Value.(ast.Ref); ok {
				visitCall(op, c[1:])
			}
		}
		return false
	})
}

// declaredVars returns the variables that e declares, without wildcards and generated variables.
func declaredVars(e *ast.Expr) ast.VarSet {
	vars := ast.NewVarSet()
	add := func(x any) {
		ast.WalkVars(x, func(v ast.Var) bool {
			if !v.IsWildcard() && !v.IsGenerated() {
				vars.Add(v)
			}
			return false
		})
	}
	switch {
	case e.IsAssignment():
		add(e.Operand(0))
	case e.IsSome():
		decl := e.Terms.(*ast.SomeDecl)
		for _, sym := range decl.Symbols {
			// `some k, v in xs` is the call member_3(k, v, xs): every operand but the collection is declared.
			// A plain `some x` only declares x for the body; without it x is still an iteration variable, so
			// removing it compiles: it is not counted.
			if call, ok := sym.Value.(ast.Call); ok && len(call) > 2 {
				for _, t := range call[1 : len(call)-1] {
					add(t)
				}
			}
		}
	}
	return vars
}

// defaultFlip: default x := true ↔ false.
func (g *generator) defaultFlip() {
	g.eachRule(func(rule *ast.Rule) {
		if !rule.Default || rule.Head.Value == nil || rule.Head.Value.Location == nil {
			return
		}
		b, ok := rule.Head.Value.Value.(ast.Boolean)
		if !ok {
			return
		}
		loc := rule.Head.Value.Location
		g.add(DefaultFlip, loc.Offset, loc.Offset+len(loc.Text), fmt.Sprint(!bool(b)))
	})
}

// position converts a byte offset to a 1-based line and column.
func position(src string, offset int) (int, int) {
	line := 1 + strings.Count(src[:offset], "\n")
	col := offset - strings.LastIndex(src[:offset], "\n")
	return line, col
}
