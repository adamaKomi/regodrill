package elsechain_test

import data.elsechain

test_admin if elsechain.level == "admin" with input as {"role": "admin", "mfa": true}

test_admin_without_mfa if elsechain.level == "guest" with input as {"role": "admin"}

test_user if elsechain.level == "user" with input as {"role": "user", "verified": true}

test_unverified_user if elsechain.level == "guest" with input as {"role": "user"}

test_guest if elsechain.level == "guest" with input as {}
