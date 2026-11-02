package roots_test

import data.roots

test_eight_allowed if roots.allow with input as {"score": 8}

test_seven_denied if not roots.allow with input as {"score": 7}
