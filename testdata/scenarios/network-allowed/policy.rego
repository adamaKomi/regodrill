package network

# With --allow-network the policy compiles. No test evaluates remote_ok, so no request is ever sent.
remote_ok if {
	input.check_remote
	http.send({"method": "get", "url": "http://127.0.0.1:1"}).status_code == 200 # 200 → 201, 199: no coverage
}

small if input.n < 3 # 3 → 4 survives, 3 → 2 killed by test_small_2
