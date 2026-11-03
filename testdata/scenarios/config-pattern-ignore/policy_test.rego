package lb_test

import data.lb

test_load_balancer if count(lb.violation) == 1 with input as {"review": {"kind": {"kind": "Service"}, "object": {"spec": {"type": "LoadBalancer"}}}}

test_node_port if count(lb.violation) == 0 with input as {"review": {"kind": {"kind": "Service"}, "object": {"spec": {"type": "NodePort"}}}}
