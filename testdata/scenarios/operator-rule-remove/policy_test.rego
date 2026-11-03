package ruleremove_test

import data.ruleremove

test_admin if ruleremove.allow with input as {"role": "admin"}

test_auditor if ruleremove.allow with input as {"role": "auditor"}

test_reasons if ruleremove.deny == {"no owner", "too big"} with input as {"size": 11}

test_small if ruleremove.size_class(3) == "small"

test_single if ruleremove.single with input as {"x": true}

test_fallback if not ruleremove.fallback with input as {}
