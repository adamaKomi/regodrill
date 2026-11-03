package authz

allow if {
	input.age >= 18 # COMP_BOUNDARY ignored by the configuration; CONST_NUMBER 18 → 19 survives (no test at 18)
	input.active # removal survives: no inactive test
}
