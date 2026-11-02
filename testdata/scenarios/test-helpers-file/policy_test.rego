package helpers_test

test_privileged_denied if denied(privileged_pod)

test_safe_allowed if not denied(safe_pod)
