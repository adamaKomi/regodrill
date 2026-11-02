package mocks

allow if {
	user_ok # survives: every test mocks user_ok as true
	input.action == "read" # killed by test_write_denied
}

# No coverage: the tests replace this rule with `with`, so they never evaluate it.
user_ok if {
	input.user.active
	input.user.verified
}
