package nottoggle_test

import data.nottoggle

test_active_user if nottoggle.allow with input as {"user": {"active": true}}

test_user_not_admin if not nottoggle.admin with input as {"role": "user", "mfa": true}

test_blocked if nottoggle.deny with input as {"user": {"blocked": true, "name": "b"}}

test_pass if nottoggle.pass with input as {"score": 9}
