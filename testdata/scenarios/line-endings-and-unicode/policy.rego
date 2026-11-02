package unicode

# CRLF line endings and multi-byte characters before the mutated tokens: patches use byte offsets.
# Ünïcödé comment: ✓ ✓ ✓
greeting := "héllo wörld ✓" if {
	input.n >= 3 # >= → > killed by test_greeting
	not input.mark == "✗" # removal survives: no test sets mark
}
