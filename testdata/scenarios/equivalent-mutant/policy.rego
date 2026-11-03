package equiv

# "a" is written twice. Dropping its first occurrence leaves {"b", "a"}: the same set, so the patched policy
# parses to the same AST as the original. That mutant is equivalent: not run, not scored. Dropping "b" is
# killed by test_b.
roles := {"a", "b", "a"}

allow if input.role in roles
