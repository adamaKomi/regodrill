package iteration

# A comparison inside `every`: killed by test_all_adults_at_18.
all_adults if {
	every u in input.users {
		u.age >= 18
	}
}

# A comparison inside a comprehension: killed by test_adult_names.
adult_names := [u.name | some u in input.users; u.age >= 18]

any_admin if {
	some u in input.users # stillborn: u becomes unsafe
	u.role == "admin" # killed by test_no_admin
}
