package mutate

import (
	"sort"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
)

// Every patch below checks that the source holds the expected token at the expected offset before it emits
// a mutant. Some AST locations are not the token itself (a negative number's location text is only "-", an
// implicit rule body points at the rule head), and a wrong patch is worse than a missing mutant.

// comparisonSymbols maps a comparison built-in to its infix symbol.
var comparisonSymbols = map[string]string{
	"gte": ">=", "gt": ">", "lte": "<=", "lt": "<", "equal": "==", "neq": "!=",
}

var (
	boundaryOf = map[string]string{">=": ">", ">": ">=", "<=": "<", "<": "<="}
	negationOf = map[string]string{"==": "!=", "!=": "==", "<": ">=", ">=": "<", ">": "<=", "<=": ">"}
)

// memberOps are the `in` operator: `x in xs` and `k, v in xs`.
var memberOps = map[string]bool{"internal.member_2": true, "internal.member_3": true}

// comparisons visits every infix comparison and membership, and emits COMP_BOUNDARY, COMP_NEGATE,
// CONST_STRING and CONST_NUMBER mutants.
func (g *generator) comparisons() {
	visit := func(op ast.Ref, operands []*ast.Term) {
		name := op.String()
		if memberOps[name] {
			// Membership: only the element being looked for is a constant worth changing (`"admin" in roles`).
			// Elements of a literal collection on the right are COLLECTION_DROP's.
			if len(operands) == 2 {
				g.constant(operands[0])
			}
			return
		}
		sym, ok := comparisonSymbols[name]
		if !ok || len(operands) != 2 {
			return
		}
		left, right := operands[0], operands[1]
		if from, ok := g.infixOperator(left, right, sym); ok {
			if r, ok := boundaryOf[sym]; ok {
				g.add(CompBoundary, from, from+len(sym), r)
			}
			g.add(CompNegate, from, from+len(sym), negationOf[sym])
		}
		g.constant(left)
		g.constant(right)
	}
	g.eachRule(func(rule *ast.Rule) {
		ast.WalkExprs(rule, func(e *ast.Expr) bool {
			if e.IsCall() {
				visit(e.Operator(), e.Operands())
			}
			return false
		})
		ast.WalkTerms(rule, func(t *ast.Term) bool {
			if c, ok := t.Value.(ast.Call); ok {
				if r, ok := c[0].Value.(ast.Ref); ok {
					visit(r, c[1:])
				}
			}
			return false
		})
	})
}

// infixOperator returns the offset of sym when it is the only token between the two operands.
func (g *generator) infixOperator(left, right *ast.Term, sym string) (int, bool) {
	if left == nil || right == nil || left.Location == nil || right.Location == nil {
		return 0, false
	}
	from, to := g.termEnd(left), right.Location.Offset
	if from < 0 || from > to || to > len(g.file.Source) {
		return 0, false
	}
	between := g.file.Source[from:to]
	if strings.TrimSpace(between) != sym {
		return 0, false
	}
	return from + strings.Index(between, sym), true
}

// constant emits CONST_STRING or CONST_NUMBER for a literal operand.
func (g *generator) constant(t *ast.Term) {
	if t == nil || t.Location == nil {
		return
	}
	switch v := t.Value.(type) {
	case ast.String:
		start, end, ok := g.stringToken(t)
		if !ok {
			return
		}
		// Keep the quotes and append a suffix inside them: "admin" → "admin_regodrill".
		lit := g.file.Source[start:end]
		g.add(ConstString, start, end, lit[:len(lit)-1]+"_regodrill"+lit[len(lit)-1:])
	case ast.Number:
		n, ok := v.Int64()
		if !ok {
			return
		}
		start, end, ok := g.numberToken(t)
		if !ok {
			return
		}
		g.add(ConstNumber, start, end, strconv.FormatInt(n+1, 10))
		g.add(ConstNumber, start, end, strconv.FormatInt(n-1, 10))
	}
}

// stringToken returns the span of a quoted string literal ("..." or `...`) at t's location.
func (g *generator) stringToken(t *ast.Term) (int, int, bool) {
	start := t.Location.Offset
	end := start + len(t.Location.Text)
	if end > len(g.file.Source) || end-start < 2 {
		return 0, 0, false
	}
	lit := g.file.Source[start:end]
	q := lit[0]
	if (q != '"' && q != '`') || lit[len(lit)-1] != q {
		return 0, 0, false
	}
	if s, err := strconv.Unquote(lit); err != nil || s != string(t.Value.(ast.String)) {
		return 0, 0, false
	}
	return start, end, true
}

// numberToken returns the span of an integer literal at t's location, read from the source: for a negative
// number the location text is only "-".
func (g *generator) numberToken(t *ast.Term) (int, int, bool) {
	src := g.file.Source
	start := t.Location.Offset
	i := start
	if i < len(src) && src[i] == '-' {
		i++
	}
	digits := i
	for i < len(src) && src[i] >= '0' && src[i] <= '9' {
		i++
	}
	if i == digits || (i < len(src) && strings.ContainsRune(".eE_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", rune(src[i]))) {
		return 0, 0, false
	}
	want, _ := t.Value.(ast.Number).Int64()
	if got, err := strconv.ParseInt(src[start:i], 10, 64); err != nil || got != want {
		return 0, 0, false
	}
	return start, i, true
}

// termEnd returns the offset just after a term, reading numbers from the source.
func (g *generator) termEnd(t *ast.Term) int {
	if _, ok := t.Value.(ast.Number); ok {
		if _, end, ok := g.numberToken(t); ok {
			return end
		}
		return -1
	}
	return t.Location.Offset + len(t.Location.Text)
}

// notToggle removes the `not` of a negated expression, or adds one to an expression of a rule body.
func (g *generator) notToggle() {
	g.eachRule(func(rule *ast.Rule) {
		if rule.Default {
			return
		}
		for r := rule; r != nil; r = r.Else {
			for _, e := range r.Body {
				if !g.ownExpr(e) || implicitTrue(e) || isTrace(e) {
					continue
				}
				start := e.Location.Offset
				if e.Negated {
					rest := g.file.Source[start:]
					if !strings.HasPrefix(rest, "not") || len(rest) < 4 || !isSpace(rest[3]) {
						continue
					}
					end := start + 3
					for end < len(g.file.Source) && isSpace(g.file.Source[end]) {
						end++
					}
					g.add(NotToggle, start, end, "")
					continue
				}
				// `not` cannot apply to a declaration, and a negated expression binds nothing: leave alone the
				// expressions whose variables nothing else binds.
				if e.IsAssignment() || e.IsEquality() || e.IsSome() || e.IsEvery() || g.needsBinding(rule, r, e) {
					continue
				}
				g.insert(NotToggle, start, "not ")
			}
		}
	})
}

// ruleRemove deletes one definition of a rule that has several in the policy files.
//
// Rego v0 can chain bodies after one head (`f(x) = y { a } { b }`): the parser makes one rule of the head and
// the first block, then one rule per extra block, whose location is the block alone. Removing an extra block
// deletes the block; removing the first deletes only its block, so the head moves to the next one.
func (g *generator) ruleRemove(definitions map[string]int) {
	rules := g.file.Module.Rules
	for i, rule := range rules {
		if rule.Default || rule.Location == nil {
			continue
		}
		g.rule = rule.Path()
		if definitions[g.rule.String()] < 2 {
			continue
		}
		start, end, ok := g.ruleExtent(rule)
		if !ok {
			continue
		}
		var next *ast.Rule
		if i+1 < len(rules) {
			next = rules[i+1]
		}
		switch {
		case isBlock(rule):
			// An extra block: delete it with the space before it, back to the end of the previous one.
			if i == 0 {
				continue
			}
			_, prevEnd, ok := g.ruleExtent(rules[i-1])
			if !ok || strings.TrimSpace(g.file.Source[prevEnd:start]) != "" {
				continue
			}
			g.add(RuleRemove, prevEnd, end, "")
		case next != nil && isBlock(next) && next.Location.Offset >= end &&
			strings.TrimSpace(g.file.Source[end:next.Location.Offset]) == "":
			// The first block of a chain: delete it up to the next block, which takes over the head.
			brace, ok := g.bodyBrace(rule, start, end)
			if !ok {
				continue
			}
			g.add(RuleRemove, brace, next.Location.Offset, "")
		default:
			// A whole rule: delete its lines, so the file keeps its shape.
			ls := strings.LastIndexByte(g.file.Source[:start], '\n') + 1
			if strings.TrimSpace(g.file.Source[ls:start]) != "" {
				continue // something else starts the line
			}
			le := end
			if nl := strings.IndexByte(g.file.Source[end:], '\n'); nl >= 0 {
				le = end + nl + 1
			} else {
				le = len(g.file.Source)
			}
			if rest := strings.TrimSpace(g.file.Source[end:le]); rest != "" && !strings.HasPrefix(rest, "#") {
				continue // something else ends the line
			}
			g.add(RuleRemove, ls, le, "")
		}
	}
	g.rule = nil
}

// isBlock reports whether a rule is an extra body block of a v0 chain: its location text is the block.
func isBlock(r *ast.Rule) bool {
	return r.Location != nil && strings.HasPrefix(string(r.Location.Text), "{")
}

// ruleExtent returns the span of a rule in the source. The location covers the whole rule, except for a
// rule without a body (`f("x", _)`), whose location is the name alone: the head's terms extend it, then the
// closing brackets that follow them.
func (g *generator) ruleExtent(r *ast.Rule) (int, int, bool) {
	src := g.file.Source
	start := r.Location.Offset
	end := start + len(r.Location.Text)
	if end > len(src) || src[start:end] != string(r.Location.Text) {
		return 0, 0, false
	}
	if isBlock(r) {
		return start, end, true
	}
	var terms []*ast.Term
	terms = append(terms, r.Head.Args...)
	terms = append(terms, r.Head.Key, r.Head.Value)
	for _, t := range terms {
		if t == nil || t.Location == nil || t.Location.File != g.file.Path {
			continue
		}
		// Generated head values (`allow` means `allow = true`) point inside the rule: they never extend it.
		if te := g.termEnd(t); te > end && te <= len(src) {
			end = te
		}
	}
	for end < len(src) && (src[end] == ')' || src[end] == ']' || src[end] == ' ' || src[end] == '\t') {
		if src[end] == ' ' || src[end] == '\t' {
			j := end
			for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
				j++
			}
			if j < len(src) && (src[j] == ')' || src[j] == ']') {
				end = j
				continue
			}
			break
		}
		end++
	}
	return start, end, true
}

// bodyBrace returns the offset of the `{` that opens the rule's body: the last `{` at nesting depth 0 before
// the first body expression. Strings and comments are skipped, so their braces don't count.
func (g *generator) bodyBrace(r *ast.Rule, start, end int) (int, bool) {
	if len(r.Body) == 0 || r.Body[0].Location == nil {
		return 0, false
	}
	first := r.Body[0].Location.Offset
	src := g.file.Source
	depth, brace := 0, -1
	for i := start; i < end && i < first; i++ {
		switch c := src[i]; c {
		case '#':
			for i < end && src[i] != '\n' {
				i++
			}
		case '"':
			for i++; i < end && src[i] != '"'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '`':
			for i++; i < end && src[i] != '`'; i++ {
			}
		case '(', '[':
			depth++
		case ')', ']', '}':
			depth--
		case '{':
			if depth == 0 {
				brace = i
			}
			depth++
		}
	}
	return brace, brace >= 0
}

// collectionDrop removes one element from a literal set or array of at least two constants.
func (g *generator) collectionDrop() {
	g.eachRule(func(rule *ast.Rule) {
		ast.WalkTerms(rule, func(t *ast.Term) bool {
			var elems []*ast.Term
			switch v := t.Value.(type) {
			case ast.Set:
				elems = v.Slice()
			case *ast.Array:
				for i := range v.Len() {
					elems = append(elems, v.Elem(i))
				}
			default:
				return false
			}
			if len(elems) < 2 || t.Location == nil || t.Location.Offset >= len(g.file.Source) {
				return false
			}
			if c := g.file.Source[t.Location.Offset]; c != '{' && c != '[' {
				return false
			}
			spans := make([][2]int, 0, len(elems))
			for _, el := range elems {
				start, end, ok := g.scalarToken(el)
				if !ok {
					return false // only collections of constants: `[x.name, x.image]` is not a list of roles
				}
				spans = append(spans, [2]int{start, end})
			}
			// A set's elements come out sorted by value; patches need them in source order.
			sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
			for i, sp := range spans {
				if i < len(spans)-1 {
					g.add(CollectionDrop, sp[0], spans[i+1][0], "") // the element, its comma and the space after
				} else {
					g.add(CollectionDrop, spans[i-1][1], sp[1], "") // the comma before the last element, and it
				}
			}
			return false
		})
	})
}

// scalarToken returns the span of a constant term: string, number, boolean or null.
func (g *generator) scalarToken(t *ast.Term) (int, int, bool) {
	if t.Location == nil {
		return 0, 0, false
	}
	switch v := t.Value.(type) {
	case ast.String:
		return g.stringToken(t)
	case ast.Number:
		if _, ok := v.Int64(); !ok {
			end := t.Location.Offset + len(t.Location.Text)
			if t.Location.Text[0] == '-' || end > len(g.file.Source) || g.file.Source[t.Location.Offset:end] != v.String() {
				return 0, 0, false
			}
			return t.Location.Offset, end, true
		}
		return g.numberToken(t)
	case ast.Boolean, ast.Null:
		start := t.Location.Offset
		end := start + len(t.Location.Text)
		if end > len(g.file.Source) || g.file.Source[start:end] != v.String() {
			return 0, 0, false
		}
		return start, end, true
	}
	return 0, 0, false
}

// quantWeaken rewrites `every x in xs { body }` as `some x in xs; body`.
func (g *generator) quantWeaken() {
	g.eachRule(func(rule *ast.Rule) {
		ast.WalkExprs(rule, func(e *ast.Expr) bool {
			ev, ok := e.Terms.(*ast.Every)
			if !ok || e.Negated || !g.ownExpr(e) {
				return false
			}
			text := func(x *ast.Location) (string, bool) {
				if x == nil || x.Offset+len(x.Text) > len(g.file.Source) || g.file.Source[x.Offset:x.Offset+len(x.Text)] != string(x.Text) {
					return "", false
				}
				return string(x.Text), true
			}
			var parts []string
			head := "some "
			if ev.Key != nil {
				k, ok := text(ev.Key.Location)
				if !ok {
					return false
				}
				head += k + ", "
			}
			v, ok1 := text(ev.Value.Location)
			d, ok2 := text(ev.Domain.Location)
			if !ok1 || !ok2 {
				return false
			}
			parts = append(parts, head+v+" in "+d)
			for _, b := range ev.Body {
				s, ok := text(b.Location)
				if !ok {
					return false
				}
				parts = append(parts, s)
			}
			g.add(QuantWeaken, e.Location.Offset, e.Location.Offset+len(e.Location.Text), strings.Join(parts, "; "))
			return false
		})
	})
}

// ownExpr reports whether e has a location in this file whose text is in the source at that offset.
func (g *generator) ownExpr(e *ast.Expr) bool {
	if e.Location == nil || e.Location.File != g.file.Path || e.Generated {
		return false
	}
	end := e.Location.Offset + len(e.Location.Text)
	return end <= len(g.file.Source) && g.file.Source[e.Location.Offset:end] == string(e.Location.Text)
}

// isTrace reports whether e is a call to print or trace: always true, with no effect on the decision, so
// removing or negating it says nothing about the tests.
func isTrace(e *ast.Expr) bool {
	if !e.IsCall() {
		return false
	}
	switch e.Operator().String() {
	case "print", "trace", "internal.print":
		return true
	}
	return false
}

// implicitTrue reports whether e is the `true` body the parser adds to a rule written without one
// (`pass := input.score >= 50`): its location is the rule head, not an expression.
func implicitTrue(e *ast.Expr) bool {
	t, ok := e.Terms.(*ast.Term)
	if !ok {
		return false
	}
	b, ok := t.Value.(ast.Boolean)
	return ok && bool(b) && string(e.Location.Text) != "true"
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
