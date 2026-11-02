package dynamic_test

import data.dynamic

test_all_pass if dynamic.failed == set() with input as {"owner": "a", "items": [1]}

test_five_items if dynamic.failed == {"small"} with input as {"owner": "a", "items": [1, 2, 3, 4, 5]}
