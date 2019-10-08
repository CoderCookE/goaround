package connectionpool

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"sync"
)

type message struct {
	health  bool
	backend string
}

type connection struct {
	healthy  bool
	messages chan message
	backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
}

func newConnection(proxy *httputil.ReverseProxy, backend string) (*connection, error) {
	conn := &connection{
		backend:  backend,
		messages: make(chan message),
		proxy:    proxy,
	}

	go conn.healthCheck()

	return conn, nil
}

func (c *connection) get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	defer c.RUnlock()
	health := c.healthy

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
		health := healthy.health
		c.Lock()
		c.healthy = health
		c.Unlock()
	}
}
