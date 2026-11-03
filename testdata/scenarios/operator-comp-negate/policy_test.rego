package negate_test

import data.negate

test_owner_allowed if negate.owner_access with input as {"resource": {"owner": "ann"}, "user": "ann"}

test_other_target_denied if negate.deny_other with input as {"target": "bob", "user": "ann"}

test_admin_allowed if negate.allow with input as {"admin": true, "level": 5}

test_age_limits if {
	negate.minor with input as {"age": 10}
	not negate.minor with input as {"age": 30}
	negate.senior with input as {"age": 40}
	not negate.senior with input as {"age": 70}
}
