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
}
