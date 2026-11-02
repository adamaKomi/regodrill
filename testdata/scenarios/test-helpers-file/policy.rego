package helpers

deny contains "privileged" if {
	some c in input.containers
	c.privileged # killed by test_privileged_denied
}
