package stats

import (
	"log"
	"net/http"
	"time"

	"github.com/CoderCookE/goaround/internal/gracefulserver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	Durations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "request_durations_seconds",
			Help:       "request latency distributions.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001, 1.0: 0.0},
		},
		[]string{"stage"},
	)

	CacheCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache",
			Help: "cache hit and misses",
		},
		[]string{"cache"},
	)

	RequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "request",
			Help: "requests",
		},
		[]string{"status"},
	)

	HealthGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backends",
			Help: "number of healthy backends",
		},
		[]string{"backends"},
	)

	AvailableConnectionsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "connections",
			Help: "number of available connections",
		},
		[]string{"connections"},
	)
)

func init() {
	prometheus.MustRegister(Durations)
	prometheus.MustRegister(CacheCounter)
	prometheus.MustRegister(HealthGauge)
	prometheus.MustRegister(AvailableConnectionsGauge)
	prometheus.MustRegister(RequestCounter)
}

func StartUp(addr string) {
	handler := promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{},
	)

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	graceful := gracefulserver.New(server)
	log.Printf("Starting Prometheus server on port %s", addr)
	err := graceful.ListenAndServe()
	if err != nil {
		log.Printf("Error starting Prometheus server: %s", err.Error())
	}
}
