package refheads

# Rules whose head is a ref. Tests query the whole `checks` object, which contains them.
checks.labels.present if {
	input.labels.owner # killed by test_missing_owner
	input.labels.team # survives: no test misses only the team
}

checks.replicas.ok if input.replicas <= 10 # <= → < killed: tests use 10 replicas

# A variable in the head: the rule path stops at `roles`.
roles[name] := role if {
	some name, role in input.users # stillborn: name and role become unsafe
	role != "none" # killed by test_none_role_dropped
}
