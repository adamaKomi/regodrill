package quant

# every → some: killed by test_one_privileged (one safe container is no longer enough).
all_safe if {
	every c in input.containers {
		c.privileged == false
	}
}

# every k, v → some k, v: survives, no test has an empty label next to a non-empty one.
all_labeled if {
	every _, v in input.labels {
		v != ""
	}
}
