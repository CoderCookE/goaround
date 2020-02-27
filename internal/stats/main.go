package stats

import (
	"flag"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	addr = flag.String("prometheus-listen-address", ":8080", "The address to listen on for HTTP requests.")
)

var (
	Durations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "request_durations_seconds",
			Help:       "request latency distributions.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"duration"},
	)

	CacheCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache",
			Help: "cache hit and misses",
		},
		[]string{"cache"},
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
}

func StartUp() {
	flag.Parse()

	http.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{},
	))

	log.Fatal(http.ListenAndServe(*addr, nil))
}
