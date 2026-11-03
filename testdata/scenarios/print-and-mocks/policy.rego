package pm

allow if {
	print("checking", input.user) # print is always true: never mutated
	is_admin(input.user) # replaced in the tests; removal and `not` killed by test_not_admin (mocked as false)
	input.action == "read"
}

# Every test replaces this function with `with`, so no test evaluates it: no coverage.
is_admin(u) if {
	u.role == "admin"
	u.active
}

# A built-in replaced in the tests: the boundary is killed by test_expired_at_deadline.
expired if time.now_ns() >= input.deadline
