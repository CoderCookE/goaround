package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/CoderCookE/goaround/internal/customflags"
	"github.com/CoderCookE/goaround/internal/pool"
	"github.com/CoderCookE/goaround/internal/stats"
)

func main() {
	portString, backends, numConns, cacert, privkey, enableCache := parseFlags()

	config := &pool.Config{
		Backends:    backends,
		NumConns:    *numConns,
		EnableCache: *enableCache,
	}

	log.Printf("Starting with conf, %s %s %d", portString, backends, *numConns)

	connectionPool := pool.New(config)
	defer connectionPool.Shutdown()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer r.Body.Close()
		connectionPool.Fetch(w, r)

		duration := time.Since(start).Seconds()
		stats.Durations.WithLabelValues("handle").Observe(duration)
	})

	go stats.StartUp()

	server := &http.Server{
		Addr:         portString,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	defer server.Close()

	var err error
	if *cacert != "" && *privkey != "" {
		err = server.ListenAndServeTLS(*cacert, *privkey)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil {
		log.Printf("ListenAndServe: %s", err)
	}
}

func parseFlags() (portString string, backends customflags.Backend, numConns *int, cacert *string, privkey *string, enableCache *bool) {
	port := flag.Int("p", 3000, "Load Balancer Listen Port (default: 3000)")
	numConns = flag.Int("n", 3, "Max number of connections per backend")

	backends = make(customflags.Backend, 0)
	flag.Var(&backends, "b", "Backend location ex: http://localhost:9000")

	cacert = flag.String("cacert", "", "cacert location")
	privkey = flag.String("privkey", "", "privkey location")

	enableCache = flag.Bool("cache", false, "Enable request cache")
	flag.Parse()
	portString = fmt.Sprintf(":%d", *port)

	return
}
