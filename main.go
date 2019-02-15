package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/codercooke/goaround/internal/connection-pool"
	"github.com/codercooke/goaround/internal/custom-flags"
)

func main() {
	portString, backends, numConns, cacert, privkey := parseFlags()

	fmt.Printf("Starting with conf, %s %s %s", portString, backends, *numConns)

	connectionPool := connectionpool.New(backends, *numConns)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionPool.Fetch(r.URL.Path, w)
	})

	server := &http.Server{
		Addr:         portString,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	defer server.Close()

	var err error

	if *cacert != "" && *privkey != "" {
		err = server.ListenAndServeTLS(*cacert, *privkey)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil {
		fmt.Sprintf("ListenAndServe: %s", err)
	}
}

func parseFlags() (portString string, backends customflags.Backend, numConns *int) {
	port := flag.Int("p", 3000, "Load Balancer Listen Port (default: 3000)")
	numConns = flag.Int("n", 3, "Max number of connections per backend")

	backends = make(customflags.Backend, 0)
	flag.Var(&backends, "b", "Backend location ex: http://localhost:9000))")

	cacert = flag.String("cacert", "", "cacert location")
	privkey = flag.String("privkey", "", "privkey location")

	flag.Parse()
	portString = fmt.Sprintf(":%d", *port)

	return
}
