package connectionpool

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
	"github.com/dgraph-io/ristretto"
)

func TestConnection(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	assertion := &assert.Asserter{T: t}

	t.Run("when in a healthy state", func(t *testing.T) {
		wg := &sync.WaitGroup{}

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

		wg.Add(1)
		conn, err := newConnection(proxy, backend, cache, wg)
		assertion.Equal(err, nil)

		assertion.False(conn.healthy)

		wg.Add(1)
		conn.messages <- message{health: true, ack: wg}
		wg.Wait()

		conn.Lock()
		health := conn.healthy
		conn.Unlock()
		assertion.True(health)

		wg.Add(1)
		conn.messages <- message{health: false, ack: wg}
		wg.Wait()

		conn.Lock()
		health = conn.healthy
		conn.Unlock()

		assertion.False(health)
	})
}
