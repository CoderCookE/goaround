package connectionpool

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
	"github.com/dgraph-io/ristretto"
)

func TestHealthCheck(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	assertion := &assert.Asserter{T: t}

	t.Run("backend returns a healthy state", func(t *testing.T) {
		backend := "http://www.google.com/"

		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: 1e7,     // number of keys to track frequency of (10M).
			MaxCost:     1 << 30, // maximum cost of cache (1GB).
			BufferItems: 64,      // number of keys per Get buffer.
		})
		assertion.Equal(err, nil)

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		conn, err := newConnection(proxy, backend, cache)
		assertion.Equal(err, nil)

		assertion.False(conn.healthy)
		conn.messages <- message{health: true}
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health := conn.healthy
		conn.Unlock()
		assertion.True(health)

		conn.messages <- message{health: false}
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health = conn.healthy
		conn.Unlock()

		assertion.False(health)
	})
}
