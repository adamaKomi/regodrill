package constnum

# 10 → 11 killed by test_eleven_items, 10 → 9 killed by test_ten_items.
quota_ok if count(input.items) <= 10

# A negative number: -2 → -1 killed by test_minus_one; -2 → -3 survives (no test at -2).
min_ok if input.n > -2

# 443 → 444 and 443 → 442: both killed by test_https.
https if input.port == 443

# Not mutated: a decimal, and a number outside a comparison.
low_ratio if input.ratio < 0.5

first_three := array.slice(input.items, 0, 3)
