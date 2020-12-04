package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spf13/viper"

	"github.com/CoderCookE/goaround/internal/gracefulserver"
	"github.com/CoderCookE/goaround/internal/pool"
	"github.com/CoderCookE/goaround/internal/stats"
)

func main() {
	parseConfig()

	config := &pool.Config{
		Backends:    viper.GetStringSlice("backends.hosts"),
		NumConns:    viper.GetInt("backends.numConns"),
		EnableCache: viper.GetBool("cache.enabled"),
	}

	connectionPool := pool.New(config)
	defer connectionPool.Shutdown()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer r.Body.Close()
		ctx := context.WithValue(context.Background(), "attempts", 1)

		connectionPool.Fetch(w, r.WithContext(ctx))

		duration := time.Since(start).Seconds()
		stats.Durations.WithLabelValues("handle").Observe(duration)
	})

	go stats.StartUp(fmt.Sprintf(":%d", viper.GetInt("metrics.port")))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", viper.GetInt("server.port")),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	defer server.Close()

	graceful := gracefulserver.New(server)
	var err error
	cacert := viper.GetString("server.cacert")
	privkey := viper.GetString("server.privkey")

	if cacert != "" && privkey != "" {
		err = graceful.ListenAndServeTLS(cacert, privkey)
	} else {
		err = graceful.ListenAndServe()
	}

	if err != nil {
		log.Printf("ListenAndServe: %s", err)
	}
}

func parseConfig() {
	configFile := flag.String("c", "/etc/goaround", "Config File Location (optional) ")
	flag.Parse()

	loadConfigFile(*configFile)
	return
}
