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

func (c *connection) get(method, route string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.backend, route)

	c.RLock()
	health := c.healthy
	c.RUnlock()

	err := errors.New("Unhealthy Node")

	if health {
		var request *http.Request
		request, err = http.NewRequest(method, url, nil)
		if err == nil {
			return c.client.Do(request)
		}
	}

	return nil, err
}

func (c *connection) healthCheck() {
	for {
		healthy := <-c.messages
		c.Lock()
		c.healthy = healthy
		c.Unlock()
	}
}
