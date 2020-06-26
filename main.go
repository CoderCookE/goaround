package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/CoderCookE/goaround/internal/customflags"
	"github.com/CoderCookE/goaround/internal/gracefulserver"
	"github.com/CoderCookE/goaround/internal/pool"
	"github.com/CoderCookE/goaround/internal/stats"
)

func main() {
	portString, metricPortString, backends, numConns, cacert, privkey, enableCache := parseFlags()

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

	go stats.StartUp(metricPortString)

	server := &http.Server{
		Addr:         portString,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	defer server.Close()

	graceful := gracefulserver.New(server)
	var err error
	if *cacert != "" && *privkey != "" {
		err = graceful.ListenAndServeTLS(*cacert, *privkey)
	} else {
		err = graceful.ListenAndServe()
	}

	if err != nil {
		log.Printf("ListenAndServe: %s", err)
	}
}

func parseFlags() (portString, metricPortString string, backends customflags.Backend, numConns *int, cacert *string, privkey *string, enableCache *bool) {
	port := flag.Int("p", 3000, "Load Balancer Listen Port (default: 3000)")
	numConns = flag.Int("n", 3, "Max number of connections per backend")

	backends = make(customflags.Backend, 0)
	flag.Var(&backends, "b", "Backend location ex: http://localhost:9000")

	cacert = flag.String("cacert", "", "cacert location")
	privkey = flag.String("privkey", "", "privkey location")

	metricPort := flag.Int("prometheus-port", 8080, "The address to listen on for HTTP requests.")
	enableCache = flag.Bool("cache", false, "Enable request cache")
	flag.Parse()
	portString = fmt.Sprintf(":%d", *port)
	metricPortString = fmt.Sprintf(":%d", *metricPort)

	return
}
