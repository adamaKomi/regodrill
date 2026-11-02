package datafiles_test

import data.datafiles

test_at_limit if datafiles.ok with input as {"items": [1, 2, 3]}

test_over_limit if not datafiles.ok with input as {"items": [1, 2, 3, 4]}

test_trusted if datafiles.trusted with input as {"image": "registry.example.com/app"}

test_untrusted if not datafiles.trusted with input as {"image": "docker.io/app"}
