package funcs

allow if is_admin(input.user)

is_admin(user) if {
	user.role == "admin" # killed by test_active_member_denied
	user.active # survives: no test has an inactive admin
}

# Two definitions of one function. < → <= makes both match for 10: a conflict error, which fails the test.
# >= → > makes none match for 10: undefined, which fails the test too.
category(n) := "small" if n < 10

category(n) := "large" if n >= 10

size := category(input.n)
