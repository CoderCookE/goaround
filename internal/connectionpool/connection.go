package connectionpool

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
)

type connection struct {
	backend  string
	healthy  bool
	client   *http.Client
	messages chan bool
	sync.RWMutex
}

func newConnection(backend string, client *http.Client) *connection {
	conn := &connection{
		backend:  backend,
		client:   client,
		messages: make(chan bool),
	}

	go conn.healthCheck()

	return conn
}

func (c *connection) get(route string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.backend, route)

	c.RLock()
	health := c.healthy
	c.RUnlock()

	if health {
		return c.client.Get(url)
	} else {
		return nil, errors.New("Unhealthy Node")
	}
}

func (c *connection) healthCheck() {
	for {
		healthy := <-c.messages
		c.Lock()
		c.healthy = healthy
		c.Unlock()
	}
}