# GoAround
A simple HTTP load balancer that
is capable of managing multiple server instances in a pool and balancing the
incoming requests across those instances. If no backend instances are avaible
a 503 error will be returned.

## Make file

### Building
```
make
```

### Testing
```
make tests
```

## Running
The provided binary includes a `shasum` file located at `./bin/shasum` to check this value on macOS you can run:
```
shasum -a 1 ./bin/goaround
```
Please ensure the `sha` matches prior to running or follow the directions above to build it yourself.

```
./bin/load-blancer -p 3000 -b http://localhost:9000 -b http//localhost:9001
```

### Flags
```
-p port: defaults to 3000
-b backend-address: may be passed multiple times
-n max number of connections per backend
-cacert location of certficate authority cert
-privkey location of private key
```

## Detailed Implemnation
This service starts a web server on a user defined port, passed via `-p` flag,
if no flag is passed the service will default to port 3000.

Backend services are passed via `-b` flags, each backend passed will created a [connection](internal/connection-pool/connection),
which are managed by a [pool](internal/connection-pool/connection).  The `connections` are pushed into a buffered channel
where they are retrieved when the `Fetch` method is called on the `pool`.  The `Fetch` method will recursively pull connections
from the channel until a request is completed successfully, or we run out of available connections.  If the backend request is successful
we copy the contents of the reponse back through to the original user request, if we are unsuccesful we return a 503 status code.  
Unsuccesful requests will cause the `connection` to be marked as degraded and pushed into a seperate channel of known degraded connections,
this channel is checked every second and healthy connections are returned to the connection pool.

The `pool` service also upon creation starts a go return that makes use of a ticker to periodically pull connections off 
our `connections channel`  and performs a `healthCheck()`, which updates the state of the connection to either "degraded" or "healthy".

## Repo contents
```
|_main.go  
|_main_test.go
|_internal
||_assert
|||_assert.go 
||_custom-flags
|||_backend.go 
||_connection-pool
|||_connection.go
|||_connection_test.go
|||_pool.go
|||_pool_test.go
```
