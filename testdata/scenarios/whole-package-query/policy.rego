package whole

allow if {
	input.method == "GET" # killed by test_post_decision
	input.path == "/public" # survives: no test sends GET to another path
}

deny contains "no path" if not input.path
