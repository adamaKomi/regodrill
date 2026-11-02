package abspath_test

import data.abspath

test_level_2 if abspath.allow with input as {"user": {"active": true, "level": 2}}

test_inactive_denied if not abspath.allow with input as {"user": {"active": false, "level": 5}}
