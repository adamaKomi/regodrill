package failing_test

import data.failing

test_ok if failing.allow with input as {"ok": true}

# Fails on the unmodified policy: nothing is mutated, exit code 2.
test_wrong if failing.allow with input as {"ok": false}
