package dynamic

checks.has_owner if input.owner
checks.small if count(input.items) < 5 # < → <= killed by test_five_items

# The rule looked up depends on a variable: data.dynamic.checks[name].
failed contains name if {
	some name in ["has_owner", "small"]
	not checks[name] # killed by test_all_pass
}
