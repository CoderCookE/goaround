package connection

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestConnection(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := []byte("hello")

		_, err := w.Write(message)
		if err != nil {
			log.Printf("Error writing: %s", err.Error())
		}
	})

	t.Run("When get is called on a healthy connection", func(t *testing.T) {
		availableServer := httptest.NewServer(availableHandler)
		backend := availableServer.URL

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, startup)

		assertion.NotEqual(conn, nil)
		assertion.Equal(err, nil)
		startup.Wait()
		conn.healthy = true

		usableProxy, err := conn.Get()
		assertion.Equal(err, nil)
		assertion.Equal(reflect.TypeOf(usableProxy).String(), "*httputil.ReverseProxy")
	})

	t.Run("starts in an unhealthy state", func(t *testing.T) {
		backend := "http://www.google.com/"

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, startup)
		assertion.Equal(err, nil)
		startup.Wait()

		assertion.False(conn.healthy)
	})

	t.Run("when passed a new health stat", func(t *testing.T) {
		backend := "http://www.google.com/"

		url, err := url.ParseRequestURI(backend)
		assertion.Equal(err, nil)

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = tr

		startup := &sync.WaitGroup{}
		startup.Add(1)
		conn := NewConnection(proxy, backend, startup)
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
