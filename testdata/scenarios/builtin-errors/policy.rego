package builtins

# A type guard before a built-in that fails on other types. Like `opa test`, OPA turns the built-in error
# into an undefined result, so for a number the guard changes nothing. In both rules, removing the length
# check survives: no test uses a short string.

# Tested with a number only: removing the guard survives (equivalent for numbers, not for arrays).
long_name if {
	is_string(input.name)
	count(input.name) > 3
}

# Also tested with an array: removing the guard is killed by test_label_list.
long_label if {
	is_string(input.label)
	count(input.label) > 3
}
