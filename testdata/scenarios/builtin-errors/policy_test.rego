package builtins_test

import data.builtins

test_long_name if builtins.long_name with input as {"name": "abcde"}

test_name_number if not builtins.long_name with input as {"name": 12345}

test_long_label if builtins.long_label with input as {"label": "abcde"}

test_label_list if not builtins.long_label with input as {"label": ["a", "b", "c", "d"]}
