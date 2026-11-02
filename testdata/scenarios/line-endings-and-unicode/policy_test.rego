package unicode_test

import data.unicode

test_greeting if unicode.greeting == "héllo wörld ✓" with input as {"n": 3}

test_no_greeting if not unicode.greeting with input as {"n": 2}
