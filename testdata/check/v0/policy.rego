package authz

allow {
	input.user.age >= 18
}

roles[r] {
	r := input.user.roles[_]
}
