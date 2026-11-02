package svc

# Both expressions are indexed: for an input that doesn't match them, OPA's rule index skips the body, so
# the body is not covered by that test even though removing either expression changes its result.
deny contains msg if {
	input.kind == "Service"
	input.type == "LoadBalancer"
	msg := "load balancers are not allowed"
}
