package manifests_test

import data.manifests

test_six_denied if manifests.deny with input as {"replicas": 6}

test_five_allowed if not manifests.deny with input as {"replicas": 5}
