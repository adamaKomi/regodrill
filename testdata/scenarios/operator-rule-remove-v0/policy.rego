package rulev0

# Rego v0 chains bodies after one head. Removing the first block moves the head to the second one; removing
# the second block deletes it.
service_account(obj) = name {
	obj.kind == "Pod" # first block removed: killed by test_pod
	name := obj.spec.serviceAccountName
} {
	obj.kind == "Deployment" # second block removed: no coverage, no test for deployments
	name := obj.spec.template.spec.serviceAccountName
}

# Definitions without a body: f(x) is f(x) = true. Removing the first is killed by test_run_as_any; the
# second is no_coverage: no test for MustRunAsNonRoot, and the index never selects it.
accept("RunAsAny", _)

accept("MustRunAsNonRoot", user) {
	user != 0
}

ok {
	accept(input.rule, input.user)
}
