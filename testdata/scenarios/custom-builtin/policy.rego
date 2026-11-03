package custom

# result.new is a built-in of a custom OPA build (as in aquasecurity/trivy-checks), not of OPA: the
# baseline does not compile, exit code 2, and the hint says why.
deny contains res if {
	input.replicas > 5
	res := result.new("too many replicas", input)
}
