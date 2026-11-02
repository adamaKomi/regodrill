package boundaries

# >= → > is killed: a test uses age 18, the boundary.
adult if input.age >= 18

# < → <= survives: no test uses size 100.
small if input.size < 100

# A comparison used as a value, not as a body expression: killed, a test uses score 50.
pass := input.score >= 50
