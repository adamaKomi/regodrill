package defaults_test

import data.defaults

test_admin_allowed if defaults.allow with input as {"user": "admin"}

test_guest_denied if not defaults.allow with input as {"user": "guest"}

test_trusted_not_denied if defaults.deny == false with input as {"trusted": true}

test_premium_limit if defaults.limit == 20 with input as {"premium": true}

test_default_limit if defaults.limit == 10 with input as {}
