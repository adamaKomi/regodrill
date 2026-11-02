package v0

test_big_container {
	count(violation) == 1 with input as {"containers": [{"name": "a", "cpu": 4}]}
}

test_small_container {
	count(violation) == 0 with input as {"containers": [{"name": "a", "cpu": 1}]}
}
