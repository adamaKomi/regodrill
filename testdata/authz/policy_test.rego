package authz_test

import data.authz

test_adult_allowed if {
	authz.allow with input as {"user": {"age": 30, "active": true, "roles": []}}
}

test_minor_denied if {
	not authz.allow with input as {"user": {"age": 10, "active": true, "roles": []}}
}

test_admin_allowed if {
	authz.allow with input as {"user": {"age": 10, "active": false, "roles": ["admin"]}}
}
