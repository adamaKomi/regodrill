package removal

allow if {
	input.user.active # killed by test_inactive_denied
	input.user.age >= 18 # removal and boundary survive: no test uses a minor or age 18
	not input.user.banned # killed by test_banned_denied
}
