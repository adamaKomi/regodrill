package table

default allow := false

allow if {
	input.age >= 18 # >= → > killed by the "adult at 18" case; removal killed by the "minor" case
	input.active # removal survives: every case is active
}
