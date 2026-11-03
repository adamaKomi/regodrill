package custom_test

import data.custom

test_deny if count(custom.deny) == 1 with input as {"replicas": 6}
