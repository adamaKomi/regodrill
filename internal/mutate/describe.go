package mutate

import (
	"fmt"
	"strings"
)

// Description documents an operator for `regodrill operators`, SARIF rules and editors.
type Description struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Example string `json:"example"`
	Risk    string `json:"risk"`
}

// Descriptions follows the order of Operators.
var Descriptions = []Description{
	{CompBoundary, "Move a comparison boundary: >= and >, <= and <", "age >= 18 → age > 18", "Off-by-one on a limit"},
	{CompNegate, "Negate a comparison: == and !=, < and >=, > and <=", "owner == user → owner != user", "Inverted ownership or identity check"},
	{ExprRemove, "Delete one expression of a rule body that has at least two", "drop input.user.active", "A forgotten condition"},
	{NotToggle, "Remove the not of an expression, or add one", "not is_blocked → is_blocked", "Inverted deny list"},
	{DefaultFlip, "Flip a boolean default value", "default allow := false → true", "Fail-open policy"},
	{RuleRemove, "Delete one definition of a rule that has several", "drop the second allow if {...}", "A whole access path is untested"},
	{ConstString, "Change a string compared or looked up with in", `"admin" → "admin_regodrill"`, "Role or action name typo"},
	{ConstNumber, "Change an integer in a comparison by one, up and down", "count(x) > 3 → count(x) > 4", "Wrong threshold"},
	{CollectionDrop, "Remove one element from a literal set or array of constants", `{"admin", "owner"} → {"admin"}`, "A privileged role is never tested"},
	{QuantWeaken, "Weaken every to some", "every c in cs { ok(c) } → some c in cs; ok(c)", `"All" checked as "at least one"`},
}

// Describe returns the description of an operator.
func Describe(id string) (Description, bool) {
	for _, d := range Descriptions {
		if d.ID == id {
			return d, true
		}
	}
	return Description{}, false
}

// Change describes a mutant's edit in words, for reports: "Changing `>=` to `>`", "Removing `x`".
func (m *Mutant) Change() string {
	switch {
	case m.Original == "":
		return fmt.Sprintf("Adding `%s`", strings.TrimSpace(m.Replacement))
	case m.Replacement == "":
		return fmt.Sprintf("Removing `%s`", Short(m.Original))
	default:
		return fmt.Sprintf("Changing `%s` to `%s`", Short(m.Original), Short(m.Replacement))
	}
}

// Short puts text on one line and cuts it to 60 characters.
func Short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 60 {
		s = string(r[:57]) + "..."
	}
	return s
}
