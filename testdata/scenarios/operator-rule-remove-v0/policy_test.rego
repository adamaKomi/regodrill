package rulev0

test_pod {
	service_account({"kind": "Pod", "spec": {"serviceAccountName": "sa"}}) == "sa"
}

test_run_as_any {
	ok with input as {"rule": "RunAsAny", "user": 0}
}
