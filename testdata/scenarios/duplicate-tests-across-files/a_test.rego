package dup_test

import data.dup

# The same test name in two files of one package: the runner renames one of them test_level#01, in an order
# that can change between compilations. regodrill turns test selection off for this project and runs every
# test, so the verdicts stay right.
test_level if not dup.allow with input as {"level": 3, "active": true}
