package refheads_test

import data.refheads

test_all_checks_pass if {
	refheads.checks == {"labels": {"present": true}, "replicas": {"ok": true}} with input as {
		"labels": {"owner": "a", "team": "b"},
		"replicas": 10,
	}
}

test_replicas_at_10 if refheads.checks.replicas.ok with input as {"replicas": 10}

test_missing_owner if not refheads.checks.labels.present with input as {"labels": {"team": "b"}}

test_none_role_dropped if refheads.roles == {"ann": "admin"} with input as {"users": {"ann": "admin", "bob": "none"}}
