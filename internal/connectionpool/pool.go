package connectionpool

import (
	"io"
	"log"
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
	tr := &http.Transport{
		ExpectContinueTimeout: 4 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          maxRequests,
		IdleConnTimeout:       5 * time.Second,
	}

	client := &http.Client{Transport: tr}

	connectionPool := &pool{
		connections: make(chan *connection, maxRequests),
	}

	var connsPerBackend int
	backendCount := len(backends)

	if backendCount > 0 {
		connsPerBackend = maxRequests / backendCount
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

		connectionPool.healthChecks = append(connectionPool.healthChecks, hc)
		go hc.Start()
	}

	return connectionPool
}

//Exported method for passing a request to a connection from the pool
//Returns a 503 status code if request is unsuccessful
func (p *pool) Fetch(path string, w http.ResponseWriter) {
	select {
	case connection := <-p.connections:
		resp, err := connection.get(path)

		defer func() {
			p.connections <- connection
		}()

		if err != nil {
			log.Printf("retrying err with request %s", err.Error())
			p.Fetch(path, w)
		} else if resp.StatusCode == http.StatusInternalServerError {
			log.Println("retrying bad status code")
			resp.Body.Close()
			p.Fetch(path, w)
		} else {
			log.Println("success")
			defer resp.Body.Close()
			io.Copy(w, resp.Body)
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
