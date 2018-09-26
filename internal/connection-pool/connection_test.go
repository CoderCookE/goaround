package connectionpool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codercooke/goaround/internal/assert"
)

func TestHealthCheck(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	client := &http.Client{Transport: tr}
	assertion := &assert.Asserter{T: t}

	t.Run("backend returns a healthy state", func(t *testing.T) {
		resChan := make(chan bool, 1)

		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &healthCheckReponse{State: "healthy", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
			resChan <- true
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		conn := newConnection(availableServer.URL, client)

		hc := healthChecker{
			client:      client,
			subscribers: []chan bool{conn.messages},
			backend:     availableServer.URL,
		}
		hc.current_health = true

		go hc.Start()
		<-resChan
		time.Sleep(5 * time.Millisecond)

		assertion.True(conn.healthy)
	})

	t.Run("backend returns a degraded state", func(t *testing.T) {
		resChan := make(chan bool, 1)
		degradedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &healthCheckReponse{State: "degraded", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
			resChan <- true
		})

		degradedServer := httptest.NewServer(degradedHandler)
		defer degradedServer.Close()

		conn := newConnection(degradedServer.URL, client)
		hc := healthChecker{
			client:      client,
			subscribers: []chan bool{conn.messages},
			backend:     degradedServer.URL,
		}
		hc.current_health = true

		go hc.Start()
		<-resChan
		time.Sleep(5 * time.Millisecond)

		assertion.False(conn.healthy)
	})
}
