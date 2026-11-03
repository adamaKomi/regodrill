package project

import (
	"regexp"

	"github.com/open-policy-agent/opa/v1/ast"
)

// OPA's parser gives the body of a one-line else clause without braces (`else := 2 if input.c >= 4`) the
// location of the whole clause, for the expression and for every term in it (OPA v1.21.3). Mutation
// operators place their patches from term locations, so they would skip these bodies, or worse.
//
// fixElseLocations re-locates such a body: it parses the text after each `if` of the clause as a body,
// and takes the first one that is equal to the parsed body (OPA's AST comparison, which ignores locations).
// The terms then get the locations of the reparsed body, shifted to the clause's place in the file. When
// no candidate matches, nothing changes, and the operators' own source checks keep skipping the body.

var (
	ifKeyword   = regexp.MustCompile(`\bif\b`)
	elseKeyword = regexp.MustCompile(`\belse\b`)
)

func fixElseLocations(mod *ast.Module, version ast.RegoVersion) {
	for _, rule := range mod.Rules {
		for r := rule.Else; r != nil; r = r.Else {
			fixBody(r.Body, version)
		}
	}
}

func fixBody(body ast.Body, version ast.RegoVersion) {
	if len(body) == 0 || body[0].Location == nil {
		return
	}
	clause := body[0].Location
	text := string(clause.Text)
	if len(text) < 4 || text[:4] != "else" {
		return // a body with braces has its own locations
	}
	for _, m := range ifKeyword.FindAllStringIndex(text, -1) {
		start := m[1]
		for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
		parsed := matchingBody(text[start:], body, version)
		if parsed == nil {
			continue
		}
		var originals, fixed []*ast.Term
		ast.WalkTerms(body, func(t *ast.Term) bool { originals = append(originals, t); return false })
		ast.WalkTerms(parsed, func(t *ast.Term) bool { fixed = append(fixed, t); return false })
		if len(originals) != len(fixed) {
			return
		}
		shift := func(l *ast.Location) *ast.Location {
			if l == nil {
				return nil
			}
			col := l.Col
			if l.Row == 1 {
				col += clause.Col - 1 + start
			}
			return &ast.Location{
				Text: l.Text, File: clause.File, Row: clause.Row + l.Row - 1, Col: col,
				Offset: clause.Offset + start + l.Offset,
			}
		}
		for i, t := range originals {
			t.Location = shift(fixed[i].Location)
		}
		for i, e := range body {
			e.Location = shift(parsed[i].Location)
		}
		return
	}
}

// matchingBody parses rest, or a prefix of it that ends before an `else` (the clause of a one-line else that
// is followed by another else holds both), and returns the first parse equal to body.
func matchingBody(rest string, body ast.Body, version ast.RegoVersion) ast.Body {
	candidates := []string{rest}
	for _, m := range elseKeyword.FindAllStringIndex(rest, -1) {
		candidates = append(candidates, rest[:m[0]])
	}
	for _, c := range candidates {
		parsed, err := ast.ParseBodyWithOpts(c, ast.ParserOptions{RegoVersion: version})
		if err == nil && parsed.Equal(body) {
			return parsed
		}
	}
	return nil
}
