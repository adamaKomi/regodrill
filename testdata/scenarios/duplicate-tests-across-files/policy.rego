package dup

allow if {
	input.level > 3 # > → >= killed (a test uses level 3); removal killed (a test uses level 1)
	input.active # removal survives: no test is inactive
}
