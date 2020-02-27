package connection

import (
	"errors"
	"net/http/httputil"
	"sync"

	"github.com/CoderCookE/goaround/internal/stats"
)

type Message struct {
	Health   bool
	Backend  string
	Proxy    *httputil.ReverseProxy
	Ack      *sync.WaitGroup
	Shutdown bool
}

type Connection struct {
	healthy  bool
	Shut     bool
	Messages chan Message
	Backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
}

func NewConnection(proxy *httputil.ReverseProxy, backend string, startup *sync.WaitGroup) *Connection {
	conn := &Connection{
		Backend:  backend,
		Messages: make(chan Message),
		proxy:    proxy,
	}

	go conn.healthCheck()
	startup.Done()

	return conn
}

func (c *Connection) Get() (*httputil.ReverseProxy, error) {
	c.Lock()
	defer c.Unlock()

	health := c.healthy
	if health && !c.Shut {
		return c.proxy, nil
	}

	return nil, errors.New("Unhealthy Node")
}

func (c *Connection) healthCheck() {
	for {
		select {
		case msg := <-c.Messages:
			c.Lock()
			if msg.Shutdown {
				c.Shutdown()
				c.Unlock()
				return
			} else {
				backend := msg.Backend
				c.healthy = msg.Health
				proxy := msg.Proxy

				if proxy != nil && c.Backend != backend {
					c.Backend = backend
					c.proxy = proxy
				}

			}

			msg.Ack.Done()
			c.Unlock()
		}
	}
}

func (c *Connection) Shutdown() {
	c.healthy = false
	c.Shut = true
	close(c.Messages)
	stats.AvailableConnectionsGauge.WithLabelValues("available").Sub(1)
}
