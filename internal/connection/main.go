package connection

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/dgraph-io/ristretto"

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
	Messages chan Message
	Backend  string
	sync.RWMutex
	proxy *httputil.ReverseProxy
	cache *ristretto.Cache
}

func NewConnection(proxy *httputil.ReverseProxy, backend string, cache *ristretto.Cache, startup *sync.WaitGroup) *Connection {
	conn := &Connection{
		Backend:  backend,
		Messages: make(chan Message),
		proxy:    proxy,
		cache:    cache,
	}

	go conn.healthCheck()

	startup.Done()

	stats.AvailableConnectionsGauge.WithLabelValues("available").Add(1)

	return conn
}

func (c *Connection) Get(w http.ResponseWriter, r *http.Request) error {
	c.RLock()
	defer c.RUnlock()

	health := c.healthy
	err := errors.New("Unhealthy Node")
	if c.cache != nil && r.Method == "GET" {
		value, found := c.cache.Get(r.URL.Path)
		if found {
			stats.CacheCounter.WithLabelValues("hit").Add(1)
			res := value.(string)
			w.Write([]byte(res))

			return nil
		}
		stats.CacheCounter.WithLabelValues("miss").Add(1)
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
			if msg.Shutdown {
			} else {
				backend := msg.Backend
				c.healthy = msg.Health
				proxy := msg.Proxy

				if proxy != nil && c.Backend != backend {
					c.Backend = backend
					c.proxy = proxy
				}

				msg.Ack.Done()
			}
			c.Unlock()
		}
	}
}

func (c *Connection) Shutdown() {
	c.Lock()
	stats.AvailableConnectionsGauge.WithLabelValues("available").Sub(1)
	close(c.Messages)
	c.Unlock()
}
