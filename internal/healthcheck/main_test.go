package healthcheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
	"github.com/CoderCookE/goaround/internal/connection"
)

func TestHealthChecker(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	client := &http.Client{Transport: tr}
	assertion := &assert.Asserter{T: t}

	t.Run("reuse notifies subscribers", func(t *testing.T) {
		resChan := make(chan connection.Message, 1)

		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &Reponse{State: "healthy", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		hc := New(
			client,
			[]chan connection.Message{resChan},
			availableServer.URL,
			false,
		)

		defer hc.Shutdown()

		blocker := make(chan bool, 1)
		var msg connection.Message
		go func() {
			for {
				select {
				case msg = <-resChan:
					if !msg.Shutdown {
						msg.Ack.Done()
						blocker <- true
					}
				}
			}
		}()

		hc = hc.Reuse("foobar", nil)

		<-blocker
		assertion.Equal(msg.Health, false)
		assertion.Equal(msg.Backend, "foobar")
	})

	t.Run("backend returns a healthy state", func(t *testing.T) {
		resChan := make(chan connection.Message, 1)

		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &Reponse{State: "healthy", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		hc := New(
			client,
			[]chan connection.Message{resChan},
			availableServer.URL,
			false,
		)

		startup := &sync.WaitGroup{}
		startup.Add(1)
		go hc.Start(startup)
		startup.Wait()
		defer hc.Shutdown()

		health := <-resChan
		assertion.True(health.Health)
	})

	t.Run("backend returns a degraded state", func(t *testing.T) {
		resChan := make(chan connection.Message, 1)

		degradedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &Reponse{State: "degraded", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
		})

		degradedServer := httptest.NewServer(degradedHandler)
		defer degradedServer.Close()

		hc := New(
			client,
			[]chan connection.Message{resChan},
			degradedServer.URL,
			true,
		)

		startup := &sync.WaitGroup{}
		startup.Add(1)
		go hc.Start(startup)
		startup.Wait()

		defer hc.Shutdown()

		health := <-resChan
		assertion.False(health.Health)
	})

	t.Run("backend returns an error", func(t *testing.T) {
		resChan := make(chan connection.Message, 1)

		degradedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		})

		degradedServer := httptest.NewServer(degradedHandler)
		degradedServer.Close()

		hc := New(
			client,
			[]chan connection.Message{resChan},
			degradedServer.URL,
			true,
		)

		startup := &sync.WaitGroup{}
		startup.Add(1)
		go hc.Start(startup)
		startup.Wait()

		defer hc.Shutdown()

		health := <-resChan
		assertion.False(health.Health)
	})
}
