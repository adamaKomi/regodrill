package nottoggle

allow if {
	not input.user.blocked # remove `not`: killed by test_active_user
	input.user.active # add `not`: killed by test_active_user
}

# Only tested with a non-admin. Adding `not` to the role check makes the rule true: killed by
# test_user_not_admin. Rule indexing skips the body for a non-admin, so input.mfa is never evaluated:
# adding `not` there is no_coverage.
admin if {
	input.role == "admin"
	input.mfa
}

deny if {
	input.user.blocked # add `not`: killed by test_blocked
	name := input.user.name # not mutated: a declaration
	name != "" # add `not`: killed by test_blocked
}

# Not mutated: a default value, and a rule without a body.
default open := false

pass := input.score > 5
