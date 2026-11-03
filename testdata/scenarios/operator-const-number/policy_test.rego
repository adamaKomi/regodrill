package constnum_test

import data.constnum

test_ten_items if constnum.quota_ok with input as {"items": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]}

test_eleven_items if not constnum.quota_ok with input as {"items": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]}

test_minus_one if constnum.min_ok with input as {"n": -1}

test_https if constnum.https with input as {"port": 443}

test_low_ratio if constnum.low_ratio with input as {"ratio": 0.1}

test_first_three if constnum.first_three == [1, 2, 3] with input as {"items": [1, 2, 3, 4]}
