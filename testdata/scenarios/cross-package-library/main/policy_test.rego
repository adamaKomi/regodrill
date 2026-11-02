package main_test

import data.main

test_registry_allowed if main.allowed_image with input as {"image": "registry.example.com/app"}

test_other_registry_denied if not main.allowed_image with input as {"image": "docker.io/app"}
