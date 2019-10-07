package connectionpool

import (
	"errors"
	"github.com/dgraph-io/ristretto"
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
	cache *ristretto.Cache
}

func newConnection(proxy *httputil.ReverseProxy, backend string, cache *ristretto.Cache) (*connection, error) {
	conn := &connection{
		backend:  backend,
		messages: make(chan bool),
		proxy:    proxy,
		cache:    cache,
	}

	go conn.healthCheck()

	return conn, nil
}

func (c *connection) get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	health := c.healthy
	c.RUnlock()

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
		healthy := <-c.messages
		c.Lock()
		c.healthy = healthy
		c.Unlock()
	}
}
