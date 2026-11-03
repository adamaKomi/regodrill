package manifests

deny if input.replicas > 5 # > → >= killed by test_five_allowed
