package authz_test

import data.authz

test_adult_allowed if authz.allow with input as {"user": {"age": 30, "active": true}}
