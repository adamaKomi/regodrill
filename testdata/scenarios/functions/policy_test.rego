package funcs_test

import data.funcs

test_admin_allowed if funcs.allow with input as {"user": {"role": "admin", "active": true}}

test_active_member_denied if not funcs.allow with input as {"user": {"role": "member", "active": true}}

test_small if funcs.size == "small" with input as {"n": 3}

test_large_at_10 if funcs.size == "large" with input as {"n": 10}
