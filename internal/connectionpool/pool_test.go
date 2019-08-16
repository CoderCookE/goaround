package connectionpool

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
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

		reader := strings.NewReader("This is a test")
		request := httptest.NewRequest("GET", "http://www.test.com/hello", reader)
		recorder := httptest.NewRecorder()
		connectionPool.Fetch(recorder, request)

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

		reader := strings.NewReader("This is a test")
		request := httptest.NewRequest("GET", "http://www.test.com/hello", reader)
		recorder := httptest.NewRecorder()
		connectionPool.Fetch(recorder, request)

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

		healthyBackend, _ := url.Parse(availableServer.URL)
		healthyConnection := &connection{
			backend:  availableServer.URL,
			healthy:  true,
			client:   client,
			messages: make(chan bool),
			proxy:    httputil.NewSingleHostReverseProxy(healthyBackend),
		}

		unhealthyBackend, _ := url.Parse(unavailableServer.URL)
		unhealthyConnection := &connection{
			backend:  unavailableServer.URL,
			healthy:  false,
			client:   client,
			messages: make(chan bool),
			proxy:    httputil.NewSingleHostReverseProxy(unhealthyBackend),
		}

		connectionPool := New([]string{}, 10)
		connectionPool.connections <- unhealthyConnection
		connectionPool.connections <- healthyConnection

		defer connectionPool.Shutdown()

		reader := strings.NewReader("This is a test")
		request := httptest.NewRequest("GET", "http://www.test.com/hello", reader)
		recorder := httptest.NewRecorder()
		connectionPool.Fetch(recorder, request)

		<-availableResChan

		result, err := ioutil.ReadAll(recorder.Result().Body)

		assertion.Equal(err, nil)
		assertion.Equal(recorder.Code, http.StatusOK)
		assertion.Equal(string(result), `{"state":"healthy","message":""}`)
	})
}
