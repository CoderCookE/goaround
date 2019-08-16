package connectionpool

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"sync"
)

type connection struct {
	healthy  bool
	messages chan bool
	backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
}

func newConnection(proxy *httputil.ReverseProxy, backend string) (*connection, error) {
	conn := &connection{
		backend:  backend,
		messages: make(chan bool),
		proxy:    proxy,
	}

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
