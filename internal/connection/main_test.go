package connection

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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

	availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message []byte
		message = []byte("hello")
		w.Write(message)
	})

	assertion := &assert.Asserter{T: t}

	t.Run("When get is called", func(t *testing.T) {

		availableServer := httptest.NewServer(availableHandler)
		backend := availableServer.URL

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

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, cache, startup)

		assertion.NotEqual(conn, nil)
		assertion.Equal(err, nil)
		startup.Wait()
		conn.healthy = true

		req := httptest.NewRequest(http.MethodGet, backend, nil)
		res := httptest.NewRecorder()
		conn.Get(res, req)

		result, err := ioutil.ReadAll(res.Result().Body)
		assertion.Equal(err, nil)
		defer res.Result().Body.Close()

		assertion.Equal(string(result), "hello")
	})

	t.Run("starts in an unhealthy state", func(t *testing.T) {
		backend := "http://www.google.com/"

		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: 1e7,     // number of keys to track frequency of (10M).
			MaxCost:     1 << 30, // maximum cost of cache (1GB).
			BufferItems: 64,      // number of keys per Get buffer.
		})
		assertion.Equal(err, nil)
		assertion.NotEqual(cache, nil)

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, cache, startup)
		assertion.Equal(err, nil)
		startup.Wait()

		assertion.False(conn.healthy)
	})

	t.Run("when passed a new health stat", func(t *testing.T) {
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

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, cache, startup)
		startup.Wait()

		wg := &sync.WaitGroup{}

		wg.Add(1)
		conn.Messages <- Message{Health: true, Ack: wg}
		wg.Wait()

		conn.Lock()
		health := conn.healthy
		conn.Unlock()
		assertion.True(health)

		wg.Add(1)
		conn.Messages <- Message{Health: false, Ack: wg}
		wg.Wait()

		conn.Lock()
		health = conn.healthy
		conn.Unlock()

		assertion.False(health)
	})
}
