package dup_test

import data.dup

test_level if dup.allow with input as {"level": 4, "active": true}

test_low if not dup.allow with input as {"level": 1, "active": true}
