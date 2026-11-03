package negate

# == → != : killed by test_owner_allowed.
owner_access if input.resource.owner == input.user

# != → == : killed by test_other_target_denied.
deny_other if input.target != input.user

# > → <= : survives. The only test is an admin, allowed by the other definition whatever the level.
allow if input.level > 3

allow if input.admin

# < → >= and <= → > : both killed by test_age_limits.
minor if input.age < 18

senior if input.age <= 65
