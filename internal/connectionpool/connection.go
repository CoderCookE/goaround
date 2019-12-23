package connectionpool

import (
	"errors"
	"github.com/dgraph-io/ristretto"
	"net/http"
	"net/http/httputil"
	"sync"
)

type message struct {
	health  bool
	backend string
	proxy   *httputil.ReverseProxy
}

type connection struct {
	healthy  bool
	messages chan message
	backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
	cache *ristretto.Cache
}

func newConnection(proxy *httputil.ReverseProxy, backend string, cache *ristretto.Cache) (*connection, error) {
	conn := &connection{
		backend:  backend,
		messages: make(chan message),
		proxy:    proxy,
		cache:    cache,
	}

	go conn.healthCheck()

	return conn, nil
}

func (c *connection) get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	defer c.RUnlock()

	health := c.healthy
	err := errors.New("Unhealthy Node")
	if c.cache != nil {
		value, found := c.cache.Get(r.URL.Path)
		if found && r.Method == "GET" {
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

func (c *connection) healthCheck() {
	for {
		msg := <-c.messages

		c.Lock()

		backend := msg.backend
		c.healthy = msg.health
		proxy := msg.proxy

		if proxy != nil && c.backend != backend {
			c.backend = backend
			c.proxy = proxy
		}

		c.Unlock()
	}
}
