package table_test

import data.table

cases := {
	"adult at 18": {"input": {"age": 18, "active": true}, "want": true},
	"minor": {"input": {"age": 17, "active": true}, "want": false},
}

# A parameterized test: one sub-result per case. The runner judges it from the trace, so this project
# runs its mutants through OPA's test runner instead of the faster path.
test_ages[name] if {
	some name, tc in cases
	table.allow == tc.want with input as tc.input
}
