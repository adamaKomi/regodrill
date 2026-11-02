package lib.text

# Only tested through data.main, in another package and directory.
has_prefix(s, prefixes) if {
	some p in prefixes # stillborn: p becomes unsafe
	startswith(s, p) # killed by test_other_registry_denied
}
