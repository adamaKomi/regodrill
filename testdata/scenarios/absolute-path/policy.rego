package abspath

allow if {
	input.user.active # killed by test_inactive_denied
	input.user.level >= 2 # >= → > killed by test_level_2; removal survives: no test has a low level
}
