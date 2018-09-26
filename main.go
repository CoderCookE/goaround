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
	portString, backends := parseFlags()
	connectionPool := connectionpool.New(backends, 5)

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
	server.ListenAndServe()
}

func parseFlags() (portString string, backends customflags.Backend) {
	port := flag.Int("p", 3000, "Load Balancer Listen Port (default: 3000)")
	backends = make(customflags.Backend, 0)
	flag.Var(&backends, "b", "Backend location ex: http://localhost:9000))")
	flag.Parse()
	portString = fmt.Sprintf(":%d", *port)

	return
}
