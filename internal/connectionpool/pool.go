package connectionpool

import (
	"log"
	"net"
	"net/http"
	"time"
)

type pool struct {
	connections  chan *connection
	healthChecks []*healthChecker
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(backends []string, maxRequests int) *pool {
	var connsPerBackend int
	backendCount := len(backends)

	if backendCount > 0 {
		connsPerBackend = maxRequests / backendCount
	}

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 30 * time.Second,
		ResponseHeaderTimeout: 50 * time.Second,
		MaxIdleConns:          maxRequests,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConnsPerHost:   connsPerBackend + 1,
	}

	client := &http.Client{Transport: tr}

	connectionPool := &pool{
		connections: make(chan *connection, maxRequests),
	}

	for _, back := range backends {
		newConnection := newConnection(back, client)
		connections := make([]chan bool, connsPerBackend)

		for i := 0; i < connsPerBackend; i++ {
			connectionPool.connections <- newConnection
			connections[i] = newConnection.messages
		}

		hc := &healthChecker{
			client:      client,
			subscribers: connections,
			backend:     newConnection.backend,
			done:        make(chan bool, 1),
		}

		hc.check()
		connectionPool.healthChecks = append(connectionPool.healthChecks, hc)
		go hc.Start()
	}

	return connectionPool
}

//Exported method for passing a request to a connection from the pool
//Returns a 503 status code if request is unsuccessful
func (p *pool) Fetch(w http.ResponseWriter, r *http.Request) {
	select {
	case connection := <-p.connections:
		defer func() {
			p.connections <- connection
		}()

		err := connection.get(w, r)
		if err != nil {
			log.Printf("retrying err with request: %s", err.Error())
			p.Fetch(w, r)
		}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (p *pool) Shutdown() {
	for _, hc := range p.healthChecks {
		hc.Shutdown()
	}
}
