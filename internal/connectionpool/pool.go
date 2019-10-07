package connectionpool

import (
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type pool struct {
	connections  chan *connection
	healthChecks []*healthChecker
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(backends []string, connsPerBackend int) *pool {
	var maxRequests int
	backendCount := int(math.Max(float64(len(backends)), float64(1)))

	if backendCount > 0 {
		maxRequests = connsPerBackend * backendCount
	}

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          maxRequests + backendCount,
		IdleConnTimeout:       120 * time.Second,
		MaxConnsPerHost:       connsPerBackend + 1,
		MaxIdleConnsPerHost:   connsPerBackend + 1,
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}

	connectionPool := &pool{
		connections: make(chan *connection, maxRequests),
	}

	poolConnections := make([]*connection, 0)

	for _, backend := range backends {
		url, err := url.ParseRequestURI(backend)
		if err != nil {
			log.Printf("error parsing backend url: %s", backend)
		} else {
			proxy := httputil.NewSingleHostReverseProxy(url)
			proxy.Transport = client.Transport

			newConnection, err := newConnection(proxy, backend)
			if err != nil {
				log.Printf("Error adding connection for: %s", backend)
			} else {
				backendConnections := make([]chan bool, connsPerBackend)

				for i := 0; i < connsPerBackend; i++ {
					poolConnections = append(poolConnections, newConnection)
					backendConnections[i] = newConnection.messages
				}

				hc := &healthChecker{
					client:      client,
					subscribers: backendConnections,
					backend:     newConnection.backend,
					done:        make(chan bool, 1),
				}

				connectionPool.healthChecks = append(connectionPool.healthChecks, hc)
				go hc.Start()
			}
		}
	}

	shuffle(poolConnections, connectionPool.connections)

	return connectionPool
}

func shuffle(conns []*connection, ch chan *connection) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(conns), func(i, j int) {
		conns[i], conns[j] = conns[j], conns[i]
	})

	for _, conn := range conns {
		ch <- conn
	}
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
