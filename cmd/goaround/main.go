package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/CoderCookE/goaround/internal/connectionpool"
	"github.com/CoderCookE/goaround/internal/customflags"
)

func main() {
	portString, backends, numConns, cacert, privkey := parseFlags()

	fmt.Printf("Starting with conf, %s %s %d", portString, backends, *numConns)

	connectionPool := connectionpool.New(backends, *numConns)
	defer connectionPool.Shutdown()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionPool.Fetch(w, r)
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

func parseFlags() (portString string, backends customflags.Backend, numConns *int, cacert *string, privkey *string) {
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
