package slow

default safe := false

safe if {
	input.n <= 10 # removal times out: the test's n makes the next line explode; boundary killed by test_ten
	count([1 | some _ in numbers.range(1, input.n); some _ in numbers.range(1, input.n)]) > 0 # > → >= survives
}
