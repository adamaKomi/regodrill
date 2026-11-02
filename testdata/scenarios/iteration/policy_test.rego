package iteration_test

import data.iteration

test_all_adults_at_18 if iteration.all_adults with input as {"users": [{"age": 18}, {"age": 40}]}

test_not_all_adults if not iteration.all_adults with input as {"users": [{"age": 17}]}

test_adult_names if iteration.adult_names == ["b"] with input as {"users": [{"name": "a", "age": 17}, {"name": "b", "age": 18}]}

test_admin if iteration.any_admin with input as {"users": [{"role": "admin"}]}

test_no_admin if not iteration.any_admin with input as {"users": [{"role": "dev"}]}
