package connectionpool

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codercooke/goaround/internal/assert"
)

func TestFetch(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	t.Run("No backends available, returns 503", func(t *testing.T) {
		connectionPool := New([]string{}, 1)
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

		recorder := httptest.NewRecorder()
		connectionPool.Fetch("", recorder)

		assertion.Equal(recorder.Code, http.StatusServiceUnavailable)
	})

	t.Run("First connection tried is degraded, Uses next connections, returning the healthy connection to the pool", func(t *testing.T) {
		unavailableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		unavailableServer := httptest.NewServer(unavailableHandler)
		defer unavailableServer.Close()

		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			healthReponse := &healthCheckReponse{State: "healthy", Message: ""}
			healthMessage, _ := json.Marshal(healthReponse)
			w.Write(healthMessage)
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		backends := []string{unavailableServer.URL, availableServer.URL}
		connectionPool := New(backends, 1)

		recorder := httptest.NewRecorder()
		connectionPool.Fetch("", recorder)

		result, err := ioutil.ReadAll(recorder.Result().Body)

		healthyNode := <-connectionPool.connections

		assertion.NotEqual(healthyNode, nil)
		assertion.True(healthyNode.healthy)

		assertion.Equal(err, nil)
		assertion.Equal(recorder.Code, http.StatusOK)
		assertion.Equal(string(result), `{"state":"healthy","message":""}`)
	})
}
