package ignores

allow if {
	input.user.active # removal killed by test_inactive_denied
	count(input.user.roles) > 0 # regodrill:ignore COMP_BOUNDARY,CONST_NUMBER -- same as >= 1
	input.user.level >= 2 # regodrill:ignore -- the gateway checks the level
}

# Only EXPR_REMOVE is ignored on the next line: the CONST_STRING mutant of "" still runs (and survives).
# regodrill:ignore-next-line EXPR_REMOVE
deny if { input.user.banned; input.user.reason != "" }

# regodrill:ignore
stale := 1 # the annotation above has no mutant on its own line: a warning

named if input.name == "a" # regodrill:ignore NOPE, CONST_STRING
