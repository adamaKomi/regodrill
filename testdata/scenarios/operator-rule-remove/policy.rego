package ruleremove

allow if input.role == "admin" # removed: killed by test_admin

allow if input.role == "owner" # removed: no coverage, no test for owners (the index never selects it)

deny contains "no owner" if not input.owner # removed: killed by test_reasons

deny contains "too big" if input.size > 10 # removed: killed by test_reasons

# Two definitions of a function: removing the first is killed by test_small, the second survives.
size_class(n) := "small" if n < 5

size_class(n) := "large" if n >= 5

# Not mutated: a single definition, and a rule whose only other definition is its default.
single if input.x

default fallback := false

fallback if input.y
