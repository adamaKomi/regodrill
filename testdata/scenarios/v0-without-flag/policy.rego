package v0noflag

# Rego v0 syntax parsed as v1 without --v0-compatible: the baseline does not compile, exit code 2.
allow {
	input.ok
}
