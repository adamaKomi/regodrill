package pm_test

import data.pm

test_read if pm.allow with pm.is_admin as true with input as {"user": "u", "action": "read"}

test_write if not pm.allow with pm.is_admin as true with input as {"user": "u", "action": "write"}

test_not_admin if not pm.allow with pm.is_admin as false with input as {"user": "u", "action": "read"}

test_expired_at_deadline if pm.expired with time.now_ns as 1000 with input as {"deadline": 1000}
