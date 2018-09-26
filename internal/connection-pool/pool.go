package connectionpool

import (
	"io"
	"log"
	"net/http"
	"time"
)

type pool struct {
	connections chan *connection
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(backends []string, maxRequests int) *pool {
	tr := &http.Transport{
		MaxIdleConns:    100,
		IdleConnTimeout: 5 * time.Second,
	}

	client := &http.Client{Transport: tr}

	connectionPool := &pool{
		connections: make(chan *connection, len(backends)),
	}

	for _, back := range backends {
		newConnection := newConnection(back, client)
		connectionPool.connections <- newConnection

		connections := make([]chan bool, maxRequests)
		for i := 0; i < maxRequests; i++ {
			connections[i] = newConnection.messages
		}

		hc := healthChecker{
			client:      client,
			subscribers: connections,
			backend:     newConnection.backend,
		}

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

		p.connections <- connection
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}
