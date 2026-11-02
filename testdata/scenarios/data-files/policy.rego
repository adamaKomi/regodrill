package datafiles

# data.limits comes from data.json, data.registries from data.yaml.
ok if count(input.items) <= data.limits.max_items # <= → < killed by test_at_limit

trusted if {
	some r in data.registries.trusted
	startswith(input.image, r) # killed by test_untrusted
}
