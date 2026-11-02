package whole_test

# The tests query the whole package, not a rule: the lookup of data.whole contains every rule in it.
test_get_decision if {
	d := data.whole with input as {"method": "GET", "path": "/public"}
	d.allow
	d.deny == set()
}

test_post_decision if {
	d := data.whole with input as {"method": "POST", "path": "/public"}
	not d.allow
}
