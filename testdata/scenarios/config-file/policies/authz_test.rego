package authz_test

import data.authz
import data.vendor.lib

test_adult if authz.allow with input as {"age": 30, "active": true}

test_minor if not authz.allow with input as {"age": 17, "active": true}

test_lib if lib.small(3)
