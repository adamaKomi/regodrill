package slow_test

import data.slow

test_ten if slow.safe with input as {"n": 10}

test_huge if not slow.safe with input as {"n": 100000}
