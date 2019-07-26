package connectionpool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codercooke/goaround/internal/assert"
)

func TestHealthChecker(t *testing.T) {
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
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		hc := healthChecker{
			client:        client,
			subscribers:   []chan bool{resChan},
			backend:       availableServer.URL,
			done:          make(chan bool),
			currentHealth: false,
		}

		go hc.Start()
		defer hc.Shutdown()

		health := <-resChan

		assertion.True(health)
	})

	t.Run("backend returns a degraded state", func(t *testing.T) {
		resChan := make(chan bool, 1)

		degradedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &healthCheckReponse{State: "degraded", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
		})

		degradedServer := httptest.NewServer(degradedHandler)
		defer degradedServer.Close()

		hc := healthChecker{
			client:        client,
			subscribers:   []chan bool{resChan},
			backend:       degradedServer.URL,
			done:          make(chan bool),
			currentHealth: true,
		}

		go hc.Start()
		defer hc.Shutdown()

		health := <-resChan

		assertion.False(health)
	})
}
