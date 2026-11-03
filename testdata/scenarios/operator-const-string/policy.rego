package conststr

# "admin" → "admin_regodrill": killed by test_admin.
allow if input.role == "admin"

# A string on the left of `in`: killed by test_root_denied.
deny if "root" in input.users

# No coverage: the only test uses another name, and rule indexing skips the body for it, so no test ever
# evaluates the comparison.
reserved if input.name == "system"

# A raw string: killed by test_api_path.
api if input.path == `/api`

# Not mutated: elements of a collection (COLLECTION_DROP's), and strings outside comparisons.
editor if input.role in {"editor", "writer"}

greeting := sprintf("hello %s", [input.name])
