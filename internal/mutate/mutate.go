// Package mutate generates mutants: one small text change applied to a policy file.
package mutate

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/adamaKomi/regodrill/internal/project"
)

// Operator IDs, as in docs/mutation-testing-spec.md §5.
const (
	CompBoundary   = "COMP_BOUNDARY"
	CompNegate     = "COMP_NEGATE"
	ExprRemove     = "EXPR_REMOVE"
	NotToggle      = "NOT_TOGGLE"
	DefaultFlip    = "DEFAULT_FLIP"
	RuleRemove     = "RULE_REMOVE"
	ConstString    = "CONST_STRING"
	ConstNumber    = "CONST_NUMBER"
	CollectionDrop = "COLLECTION_DROP"
	QuantWeaken    = "QUANT_WEAKEN"
)

// Operators lists the operators implemented by this build, in the order of the specification. All of them
// are on by default. LOGIC_SWAP waits for the OPA version that has the `and` and `or` keywords.
var Operators = []string{
	CompBoundary, CompNegate, ExprRemove, NotToggle, DefaultFlip, RuleRemove,
	ConstString, ConstNumber, CollectionDrop, QuantWeaken,
}

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
	// RuleLine is the line where the enclosing definition starts: each definition of an incremental rule,
	// function or else chain has its own. Editors group mutants by it (a CodeLens per definition).
	RuleLine int `json:"ruleLine"`
	// Range is the replaced text, end exclusive. An insertion has an empty range.
	Range Range `json:"range"`
}

// Range is a span of source text. Lines and columns are 1-based; columns count UTF-16 code units, like
// editors and SARIF do, so they equal byte columns on ASCII lines.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position is a line and column in a source file.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Apply returns the source of file with the mutation applied.
func (m *Mutant) Apply(src string) string {
	return src[:m.Start] + m.Replacement + src[m.End:]
}

// Generate returns the mutants of every non-test file, in a deterministic order.
func Generate(p *project.Project, enabled map[string]bool) []*Mutant {
	// Definitions of each rule across the policy files, for RULE_REMOVE. Default definitions don't count:
	// removing one is not removing an access path.
	definitions := map[string]int{}
	// Rule names by package, test files included: a body can refer to any rule of its package.
	ruleNames := map[string]map[ast.Var]bool{}
	for _, f := range p.Files {
		pkg := f.Module.Package.Path.String()
		if ruleNames[pkg] == nil {
			ruleNames[pkg] = map[ast.Var]bool{}
		}
		for _, r := range f.Module.Rules {
			if ref := r.Head.Ref(); len(ref) > 0 {
				if v, ok := ref[0].Value.(ast.Var); ok {
					ruleNames[pkg][v] = true
				}
			}
			if !f.IsTest && !r.Default {
				definitions[r.Path().String()]++
			}
		}
	}
	var out []*Mutant
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		globals := map[ast.Var]bool{}
		for v := range ruleNames[f.Module.Package.Path.String()] {
			globals[v] = true
		}
		for _, imp := range f.Module.Imports {
			if name := imp.Name(); name != "" {
				globals[name] = true
			}
		}
		g := &generator{file: f, enabled: enabled, globals: globals}
		g.comparisons()
		g.exprRemove()
		g.notToggle()
		g.defaultFlip()
		g.ruleRemove(definitions)
		g.collectionDrop()
		g.quantWeaken()
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
	// ruleLine is the 1-based line where the definition being visited starts (its head).
	ruleLine int
	// globals are the names a rule body can use without binding them: the rules of the file's package (in
	// every file of the package) and the file's imports.
	globals map[ast.Var]bool
	mutants []*Mutant
}

// eachRule calls fn for every rule of the file, with g.rule set to its path.
func (g *generator) eachRule(fn func(*ast.Rule)) {
	for _, rule := range g.file.Module.Rules {
		g.rule = rule.Path()
		g.ruleLine = rule.Location.Row
		fn(rule)
	}
	g.rule = nil
}

// insert adds a mutant that inserts text at offset without removing anything.
func (g *generator) insert(op string, offset int, text string) {
	if !g.enabled[op] || offset < 0 || offset > len(g.file.Source) {
		return
	}
	line, col := position(g.file.Source, offset)
	g.mutants = append(g.mutants, &Mutant{
		Operator: op, File: g.file.Path, Line: line, Column: col,
		Start: offset, End: offset, Original: "", Replacement: text,
		Rule: g.rule.String(), RuleRef: g.rule, RuleLine: g.ruleLine,
		Range: Range{Position{line, col}, Position{line, col}},
	})
}

func (g *generator) add(op string, start, end int, replacement string) {
	if !g.enabled[op] || start < 0 || end > len(g.file.Source) || start >= end {
		return
	}
	line, col := position(g.file.Source, start)
	endLine, endCol := position(g.file.Source, end)
	g.mutants = append(g.mutants, &Mutant{
		Operator: op, File: g.file.Path, Line: line, Column: col,
		Start: start, End: end, Original: g.file.Source[start:end], Replacement: replacement,
		Rule: g.rule.String(), RuleRef: g.rule, RuleLine: g.ruleLine,
		Range: Range{Position{line, col}, Position{endLine, endCol}},
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
				if e.Location == nil || e.Location.File != g.file.Path || isTrace(e) || g.declaresUsedVar(rule, r, e) {
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

// declaresUsedVar reports whether removing e from the body of r leaves a variable unbound: e binds it
// (`x := ...`, `some x in ...`, `x = ...`, `items[x]`), the head or another expression uses it, and no
// other expression could bind it instead. Such a mutant never compiles (rego_unsafe_var_error), so it is
// not generated. The check is conservative: when in doubt, the mutant is kept and the compiler decides.
func (g *generator) declaresUsedVar(top, r *ast.Rule, e *ast.Expr) bool {
	declared := g.declaredVars(e)
	if len(declared) == 0 {
		return false
	}
	used := ast.NewVarSet()
	ast.WalkVars(r.Head, func(v ast.Var) bool {
		used.Add(v)
		return false
	})
	for _, other := range r.Body {
		if other != e {
			ast.WalkVars(other, func(v ast.Var) bool {
				used.Add(v)
				return false
			})
		}
	}
	bindable := g.bindableElsewhere(top, r, e)
	for v := range declared {
		if used.Contains(v) && !bindable.Contains(v) {
			return true
		}
	}
	return false
}

// needsBinding reports whether `not e` could not compile: a negated expression binds nothing, so every
// local variable in it must be bound by another expression or a function parameter. A wildcard never is.
func (g *generator) needsBinding(top, r *ast.Rule, e *ast.Expr) bool {
	bindable := g.bindableElsewhere(top, r, e)
	vis := ast.NewVarVisitor().WithParams(ast.VarVisitorParams{SkipRefCallHead: true, SkipClosures: true, SkipWithTarget: true})
	vis.Walk(e)
	for v := range vis.Vars() {
		if v.IsWildcard() {
			return true
		}
		if v.IsGenerated() || g.isGlobal(v) {
			continue
		}
		if !bindable.Contains(v) {
			return true
		}
	}
	return false
}

// bindableElsewhere returns the variables that the function parameters or the other expressions of the body
// could bind: assignments, `some ... in`, unification patterns, reference keys (`items[x]` iterates) and
// possible output arguments.
func (g *generator) bindableElsewhere(top, r *ast.Rule, e *ast.Expr) ast.VarSet {
	out := ast.NewVarSet()
	for _, h := range []*ast.Rule{top, r} {
		for _, arg := range h.Head.Args {
			ast.WalkVars(arg, func(v ast.Var) bool {
				out.Add(v)
				return false
			})
		}
	}
	for _, other := range r.Body {
		if other == e {
			continue
		}
		bindingVars(other, out)
		for v := range g.declaredVars(other) {
			out.Add(v)
		}
	}
	return out
}

// bindingVars adds to out the variables that e could bind on its own: the patterns of a unification
// (`x = y`, `[a, b] = pair`), keys of a reference (`input.items[x]`, which iterates), and the last argument
// of a call that may be an output argument (`count(xs, n)`, or any user function with two arguments or
// more). The head of a reference (`pod` in `pod.spec`) is never bound by it.
func bindingVars(e *ast.Expr, out ast.VarSet) {
	if e.Negated {
		return // `not checks[name]` binds nothing: name must already be bound
	}
	addVars := func(x any) {
		ast.WalkVars(x, func(v ast.Var) bool {
			out.Add(v)
			return false
		})
	}
	if e.IsEquality() {
		for _, t := range e.Operands() {
			patternVars(t, out)
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

// patternVars adds the variables that a unification operand can bind: a variable, or the variables inside an
// array or object pattern. References and calls bind nothing as a whole.
func patternVars(t *ast.Term, out ast.VarSet) {
	switch v := t.Value.(type) {
	case ast.Var:
		out.Add(v)
	case *ast.Array:
		for i := range v.Len() {
			patternVars(v.Elem(i), out)
		}
	case ast.Object:
		v.Foreach(func(_, val *ast.Term) { patternVars(val, out) })
	}
}

// declaredVars returns the local variables that e binds, without wildcards, generated variables and
// global names (rules, imports, input, data): the left side of `:=`, the variables of `some ... in`, the
// variable operands of a unification, and the keys of a reference used as a whole expression (`pods[pod]`).
func (g *generator) declaredVars(e *ast.Expr) ast.VarSet {
	vars := ast.NewVarSet()
	if e.Negated {
		return vars
	}
	// `:=` and `some` always declare a local variable, even one named like a rule (it shadows the rule). In a
	// unification or a reference key, a global name is a reference, not a binding.
	add := func(x any, locals bool) {
		ast.WalkVars(x, func(v ast.Var) bool {
			if !v.IsWildcard() && !v.IsGenerated() && (locals || !g.isGlobal(v)) {
				vars.Add(v)
			}
			return false
		})
	}
	switch {
	case e.IsAssignment():
		add(e.Operand(0), true)
	case e.IsSome():
		decl := e.Terms.(*ast.SomeDecl)
		for _, sym := range decl.Symbols {
			// `some k, v in xs` is the call member_3(k, v, xs): every operand but the collection is declared.
			// A plain `some x` only declares x for the body; without it x is still an iteration variable, so
			// removing it compiles: it is not counted.
			if call, ok := sym.Value.(ast.Call); ok && len(call) > 2 {
				for _, t := range call[1 : len(call)-1] {
					add(t, true)
				}
			}
		}
	case e.IsEquality():
		for _, t := range e.Operands() {
			if v, ok := t.Value.(ast.Var); ok {
				add(ast.VarTerm(string(v)), false)
			}
		}
	default:
		if t, ok := e.Terms.(*ast.Term); ok && !e.Negated {
			if ref, ok := t.Value.(ast.Ref); ok {
				for _, k := range ref[1:] {
					if v, ok := k.Value.(ast.Var); ok {
						add(ast.VarTerm(string(v)), false)
					}
				}
			}
		}
	}
	return vars
}

// isGlobal reports whether a variable name refers to something outside the rule: a rule of the package (in
// any file), an import, input or data.
func (g *generator) isGlobal(v ast.Var) bool {
	return v == "input" || v == "data" || g.globals[v]
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

// position converts a byte offset to a 1-based line and column, the column in UTF-16 code units.
func position(src string, offset int) (int, int) {
	line := 1 + strings.Count(src[:offset], "\n")
	lineStart := strings.LastIndex(src[:offset], "\n") + 1
	col := 1
	for _, r := range src[lineStart:offset] {
		col += len(utf16.Encode([]rune{r}))
	}
	return line, col
}
