package connectionpool

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type connection struct {
	backend  string
	healthy  bool
	messages chan bool
	sync.RWMutex
	client *http.Client
	proxy  *httputil.ReverseProxy
}

func newConnection(backend string, client *http.Client) (*connection, error) {
	url, err := url.Parse(backend)
	if err != nil {
		return nil, err
	}

	conn := &connection{
		backend:  backend,
		client:   client,
		messages: make(chan bool),
		proxy:    httputil.NewSingleHostReverseProxy(url),
	}

	conn.proxy.Transport = client.Transport

	go conn.healthCheck()

	return conn, nil
}

func (c *connection) get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	health := c.healthy
	c.RUnlock()

	err := errors.New("Unhealthy Node")
	if health {
		c.proxy.ServeHTTP(w, r)
		return nil
	}

	return err
}

func (c *connection) healthCheck() {
	for {
		healthy := <-c.messages
		c.Lock()
		c.healthy = healthy
		c.Unlock()
	}
}
