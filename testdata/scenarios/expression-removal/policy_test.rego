package removal_test

import data.removal

test_adult_allowed if removal.allow with input as {"user": {"active": true, "age": 30}}

test_inactive_denied if not removal.allow with input as {"user": {"active": false, "age": 30}}

test_banned_denied if not removal.allow with input as {"user": {"active": true, "age": 30, "banned": true}}
