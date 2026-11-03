package collections_test

import data.collections

test_admin if collections.privileged with input as {"role": "admin"}

test_guest if not collections.privileged with input as {"role": "guest"}

test_https if collections.port_ok with input as {"port": 443}

test_teams if collections.teams == {"blue", "red"}

test_only if collections.only with input as {"x": "only"}

test_mixed if collections.mixed with input as {"x": "a", "y": "b"}
