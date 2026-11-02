package elsechain

level := "admin" if {
	input.role == "admin" # survives: no test has mfa without the admin role
	input.mfa # killed by test_admin_without_mfa
} else := "user" if {
	input.role == "user" # survives: no test is verified without the user role
	input.verified # killed by test_unverified_user
} else := "guest"
