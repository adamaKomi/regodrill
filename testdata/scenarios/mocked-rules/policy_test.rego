package mocks_test

import data.mocks

test_read_allowed if mocks.allow with mocks.user_ok as true with input as {"action": "read"}

test_write_denied if not mocks.allow with mocks.user_ok as true with input as {"action": "write"}
