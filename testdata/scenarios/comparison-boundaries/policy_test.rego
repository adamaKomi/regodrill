package boundaries_test

import data.boundaries

test_adult_at_18 if boundaries.adult with input as {"age": 18}

test_minor if not boundaries.adult with input as {"age": 17}

test_small if boundaries.small with input as {"size": 10}

test_big if not boundaries.small with input as {"size": 1000}

test_pass_at_50 if boundaries.pass with input as {"score": 50}

test_fail if boundaries.pass == false with input as {"score": 10}

# Skipped tests are not part of the baseline.
todo_test_later if false
