package connectionpool

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestFetch(t *testing.T) {
	assertion := &assert.Asserter{T: t}
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	client := &http.Client{Transport: tr}

	t.Run("No backends available, returns 503", func(t *testing.T) {
		connectionPool := New([]string{}, 1)
		defer connectionPool.Shutdown()

		recorder := httptest.NewRecorder()
		connectionPool.Fetch("", recorder)

		assertion.Equal(recorder.Code, http.StatusServiceUnavailable)
	})

	t.Run("none of the instances are healthy, should return an HTTP 503 response code", func(t *testing.T) {
		unavailableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		unavailableServer := httptest.NewServer(unavailableHandler)
		defer unavailableServer.Close()

		backends := []string{unavailableServer.URL}
		connectionPool := New(backends, 1)
		defer connectionPool.Shutdown()

		recorder := httptest.NewRecorder()
		connectionPool.Fetch("", recorder)

		assertion.Equal(recorder.Code, http.StatusServiceUnavailable)
	})

	t.Run("First connection tried is degraded, Uses next connections", func(t *testing.T) {
		availableResChan := make(chan bool, 1)
		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &healthCheckReponse{State: "healthy", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			availableResChan <- true
			w.Write(healthMessage)
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		unavailableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		unavailableServer := httptest.NewServer(unavailableHandler)
		defer unavailableServer.Close()

		healthyConnection := &connection{
			backend:  availableServer.URL,
			healthy:  true,
			client:   client,
			messages: make(chan bool),
		}

		unhealthyConnection := &connection{
			backend:  unavailableServer.URL,
			healthy:  false,
			client:   client,
			messages: make(chan bool),
		}

		connectionPool := New([]string{}, 10)
		connectionPool.connections <- unhealthyConnection
		connectionPool.connections <- healthyConnection

		defer connectionPool.Shutdown()

		recorder := httptest.NewRecorder()
		connectionPool.Fetch("/hello", recorder)

		<-availableResChan

		result, err := ioutil.ReadAll(recorder.Result().Body)

		assertion.Equal(err, nil)
		assertion.Equal(recorder.Code, http.StatusOK)
		assertion.Equal(string(result), `{"state":"healthy","message":""}`)
	})
}
