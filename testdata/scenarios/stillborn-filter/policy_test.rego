package stillborn_test

import data.stillborn

test_label if stillborn.label == "x" with input as {"key": "k", "guard": true, "labels": {"k": "x", "j": "y"}}

test_annotation if {
	stillborn.annotation == "one" with input as {"default_name": "a", "annotations": {"a": "one", "b": "two"}}
}

test_title if stillborn.title == "t" with input as {"title": "t"}

test_files if stillborn.files == {"f.json"} with input as {"a": true, "b": true}

test_level if stillborn.level == 3 with input as {"level": 3}
