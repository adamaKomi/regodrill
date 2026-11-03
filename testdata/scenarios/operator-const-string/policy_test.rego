package conststr_test

import data.conststr

test_admin if conststr.allow with input as {"role": "admin"}

test_root_denied if conststr.deny with input as {"users": ["root", "ann"]}

test_not_reserved if not conststr.reserved with input as {"name": "ann"}

test_api_path if conststr.api with input as {"path": "/api"}

test_editor if conststr.editor with input as {"role": "editor"}

test_greeting if conststr.greeting == "hello ann" with input as {"name": "ann"}
