package multidef

reasons contains "too many replicas" if input.replicas > 3 # > → >= killed by test_three_replicas_ok
