package connection

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/dgraph-io/ristretto"
)

type Message struct {
	Health  bool
	Backend string
	Proxy   *httputil.ReverseProxy
	Ack     *sync.WaitGroup
}

type Connection struct {
	healthy  bool
	Messages chan Message
	Backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
	cache *ristretto.Cache
}

func NewConnection(proxy *httputil.ReverseProxy, backend string, cache *ristretto.Cache, startup *sync.WaitGroup) (*Connection, error) {
	conn := &Connection{
		Backend:  backend,
		Messages: make(chan Message),
		proxy:    proxy,
		cache:    cache,
	}

	go conn.healthCheck()

	startup.Done()

	return conn, nil
}

func (c *Connection) Get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	defer c.RUnlock()

	health := c.healthy
	err := errors.New("Unhealthy Node")
	if c.cache != nil && r.Method == "GET" {
		value, found := c.cache.Get(r.URL.Path)
		if found {
			res := value.(string)
			w.Write([]byte(res))

			return nil
		}
	}

	if health {
		c.proxy.ServeHTTP(w, r)
		return nil
	}

	return err
}

func (c *Connection) healthCheck() {
	for {
		select {
		case msg := <-c.Messages:
			c.Lock()
			backend := msg.Backend
			c.healthy = msg.Health
			proxy := msg.Proxy

			if proxy != nil && c.Backend != backend {
				c.Backend = backend
				c.proxy = proxy
			}

			msg.Ack.Done()
			c.Unlock()
		}
	}
}

func (c *Connection) Shutdown() {
	close(c.Messages)
}
