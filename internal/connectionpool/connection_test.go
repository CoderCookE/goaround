package connectionpool

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestHealthCheck(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	assertion := &assert.Asserter{T: t}

	t.Run("backend returns a healthy state", func(t *testing.T) {
		backend := "http://www.google.com/"

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		conn, err := newConnection(proxy, backend)
		assertion.Equal(err, nil)

		assertion.False(conn.healthy)
		conn.messages <- true
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health := conn.healthy
		conn.Unlock()
		assertion.True(health)

		conn.messages <- false
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health = conn.healthy
		conn.Unlock()

		assertion.False(health)
	})
}
