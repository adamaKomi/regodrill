package ignores_test

import data.ignores

test_allowed if ignores.allow with input as {"user": {"active": true, "roles": ["a"], "level": 3}}

test_inactive_denied if not ignores.allow with input as {"user": {"active": false, "roles": ["a"], "level": 3}}

test_banned if ignores.deny with input as {"user": {"banned": true, "reason": "spam"}}

test_named if ignores.named with input as {"name": "a"}

test_stale if ignores.stale == 1
