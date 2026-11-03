package authz

default allow := false

allow if {
	input.user.age >= 18
	input.user.active
}
