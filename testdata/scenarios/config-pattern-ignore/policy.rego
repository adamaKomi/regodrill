package lb

violation contains msg if {
	input.review.kind.kind == "Service" # every mutant of this line is ignored by the pattern
	input.review.object.spec.type == "LoadBalancer" # removal, == → != and the string: killed by the tests
	msg := "load balancers are not allowed"
}
