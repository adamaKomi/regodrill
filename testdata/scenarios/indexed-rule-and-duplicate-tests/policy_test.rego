package svc_test

import data.svc

test_load_balancer_denied if {
	count(svc.deny) == 1 with input as {"kind": "Service", "type": "LoadBalancer"}
}

# Two tests with the same name: the test runner renames the second one test_allowed#01.
test_allowed if {
	count(svc.deny) == 0 with input as {"kind": "Service", "type": "NodePort"}
}

test_allowed if {
	count(svc.deny) == 0 with input as {"kind": "Pod", "type": "LoadBalancer"}
}
