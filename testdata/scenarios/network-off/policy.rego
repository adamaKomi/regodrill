package network

# http.send is off by default, so that a mutant can never send a request: the baseline does not compile,
# exit code 2, with a hint about --allow-network.
remote_ok if {
	input.check_remote
	http.send({"method": "get", "url": "http://127.0.0.1:1"}).status_code == 200
}

small if input.n < 3
