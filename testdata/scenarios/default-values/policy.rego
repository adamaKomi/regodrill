package defaults

# Killed: a test evaluates the default value.
default allow := false

allow if input.user == "admin"

# No coverage: every test makes the other definition match, so no test ever sees the default value.
default deny := true

deny := false if input.trusted

# Not a boolean: DEFAULT_FLIP does not apply.
default limit := 10

limit := 20 if input.premium
