package helpers_test

# Fixtures and helpers for the tests: no test_ rule here, but the _test.rego suffix makes it a test file.
# It must not be mutated.
privileged_pod := {"containers": [{"name": "a", "privileged": true}, {"name": "b"}]}

safe_pod := {"containers": [{"name": "a"}]}

denied(pod) if {
	count(data.helpers.deny) > 0 with input as pod
	true
}
