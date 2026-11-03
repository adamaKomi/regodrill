package elseoneline_test

import data.elseoneline

test_gold_at_100 if elseoneline.tier == "gold" with input as {"points": 100}

test_silver_at_50 if elseoneline.tier == "silver" with input as {"points": 50}

test_bronze if elseoneline.tier == "bronze" with input as {"points": 10}
