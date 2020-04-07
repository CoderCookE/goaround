[![Build Status](https://travis-ci.org/CoderCookE/goaround.svg?branch=master)](https://travis-ci.org/CoderCookE/goaround)
[![Maintainability](https://api.codeclimate.com/v1/badges/60d01111dd41a66baae3/maintainability)](https://codeclimate.com/github/CoderCookE/goaround/maintainability)

# GoAround
A simple HTTP load balancer that
is capable of managing multiple server instances in a pool and balancing the
incoming requests across those instances. If no backend instances are available
a 503 error will be returned.

## Make file

### Building
```
make
```

### Testing
```
make test
```

## Running
The provided binary includes a `shasum` file located at `./bin/shasum` to check this value on macOS you can run:
```
shasum -a 1 ./bin/goaround
```
Please ensure the `sha` matches prior to running or follow the directions above to build it yourself.

```
sudo ./bin/goaround -p 443 -b http://127.0.0.1:2702 -cacert /Users/ecook/cacert.pem -privkey /Users/ecook/privkey.pem
```

### Flags
```
-p port: defaults to 3000
-b backend-address: may be passed multiple times
-n max number of connections per backend
-cacert location of certficate authority cert
-privkey location of private key
-cache enabled cache for get requests
-prometheus-port defaults to 8080
```

### Flags
Metrics are created using promethus, They can be found at `localhost:8080/metrics` or whatever ports is specified via `-prometheus-port`

## Updating backends via unix socket
Pass a comma separated list of all backends;
```
echo "http://localhost:3000,http://localhost:3001" | nc -U /tmp/goaround.sock

```
The backends previously configured will be removed and replaced with only the ones passed in the updated list.

## Detailed Implementation
This service starts a web server on a user defined port, passed via `-p` flag,
if no flag is passed the service will default to port 3000.

If you pass both a `cacert` and `privkey`, the server will terminate ssl connections.

Backend services are passed via `-b` flags, each backend passed will created a [connection](internal/connection/main.go),
which are managed by a [pool](internal/pool/main.go).  The `connections` are pushed into a buffered channel
where they are retrieved when the `Fetch` method is called on the `pool`.  The `Fetch` method will recursively pull connections
from the channel until a request is completed successfully, or we run out of available connections.

Connections subscribe to a health check channel, which is pushed to if their is a change in health status for the backend. Backend
services are assumed to have a `/health` endpoint, which will return a 200 response code.   Other response codes you wish be considered
healthy must return the body in the form `{"state": "healthy", "message": ""}`

## Included Packages:
[https://github.com/dgraph-io/ristretto](https://github.com/dgraph-io/ristretto)
