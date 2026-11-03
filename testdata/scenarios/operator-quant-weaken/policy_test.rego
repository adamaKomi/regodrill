package quant_test

import data.quant

test_all_safe if quant.all_safe with input as {"containers": [{"privileged": false}, {"privileged": false}]}

test_one_privileged if not quant.all_safe with input as {"containers": [{"privileged": false}, {"privileged": true}]}

test_labeled if quant.all_labeled with input as {"labels": {"team": "a", "owner": "b"}}
