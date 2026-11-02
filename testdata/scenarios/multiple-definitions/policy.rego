package multidef

# Two definitions of one complete rule with the same value. When the first matches, OPA may stop early and
# skip the second, so tests that only satisfy the first do not cover it.
allow if input.role == "admin"

allow if input.level >= 5 # >= → > killed by test_level_5

# An incremental set split over two files: see more.rego.
reasons contains "no owner" if not input.owner
