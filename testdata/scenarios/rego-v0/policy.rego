package v0

# Rego v0 syntax: no `if`, `contains`, or `in` keyword.
violation[{"msg": msg}] {
	container := input.containers[_] # stillborn: container becomes unsafe
	container.cpu > 2 # > → >= survives: no test uses 2; removal killed by test_small_container
	msg := sprintf("%v uses too much CPU", [container.name]) # stillborn: msg is unsafe in the head
}
