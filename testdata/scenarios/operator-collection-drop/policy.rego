package collections

# Drop "admin": killed by test_admin. Drop "owner": survives, no test for owners.
privileged if input.role in {"admin", "owner"}

# Only 443 is tested: dropping 80 or 8080 survives, dropping 443 is killed by test_https.
allowed_ports := [80, 443, 8080]

port_ok if input.port in allowed_ports

# A multi-line set with a trailing comma: dropping either element is killed by test_teams.
teams := {
	"blue",
	"red",
}

# Not mutated: one element only, and elements that are not constants.
only if input.x in {"only"}

mixed if input.x in [input.y, "a"]
