package multidef_test

import data.multidef

test_admin if multidef.allow with input as {"role": "admin", "level": 9}

test_level_5 if multidef.allow with input as {"level": 5}

test_low_level if not multidef.allow with input as {"level": 4}

test_reasons if multidef.reasons == {"no owner", "too many replicas"} with input as {"replicas": 4}

test_three_replicas_ok if multidef.reasons == set() with input as {"owner": "a", "replicas": 3}
