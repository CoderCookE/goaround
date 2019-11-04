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

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestFetch(t *testing.T) {
	assertion := &assert.Asserter{T: t}
	t.Run("With cache", func(t *testing.T) {
		t.Run("Fetches from cache", func(t *testing.T) {
			callCount := 0
			availableResChan := make(chan bool, 1)
			availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message []byte

				if r.URL.Path == "/health" {
					healthReponse := &healthCheckReponse{State: "healthy", Message: ""}
					message, _ = json.Marshal(healthReponse)

					availableResChan <- true
				}

				if r.URL.Path == "/foo" {
					callCount += 1
					message = []byte("hello")
				}

				w.Write(message)
			})

			availableServer := httptest.NewServer(availableHandler)
			defer availableServer.Close()

			backends := []string{availableServer.URL}
			config := &Config{
				Backends:    backends,
				NumConns:    10,
				EnableCache: true,
			}

			connectionPool := New(config)
			defer connectionPool.Shutdown()
			<-availableResChan
			for i := 0; i < 5; i++ {
				reader := strings.NewReader("This is a test")
				request := httptest.NewRequest("GET", "http://www.test.com/foo", reader)
				recorder := httptest.NewRecorder()
				connectionPool.Fetch(recorder, request)
				result, err := ioutil.ReadAll(recorder.Result().Body)
				assertion.Equal(err, nil)
				assertion.Equal(recorder.Code, http.StatusOK)
				assertion.Equal(string(result), "hello")
				assertion.Equal(callCount, 1)
			}
		})
	})

	t.Run("No cache", func(t *testing.T) {
		t.Run("No backends available, returns 503", func(t *testing.T) {
			config := &Config{
				Backends: []string{},
				NumConns: 1,
			}
			connectionPool := New(config)
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
			config := &Config{
				Backends: backends,
				NumConns: 1,
			}

			connectionPool := New(config)
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

			healthyBackend, _ := url.ParseRequestURI(availableServer.URL)
			healthyConnection := &connection{
				backend:  availableServer.URL,
				healthy:  true,
				messages: make(chan bool),
				proxy:    httputil.NewSingleHostReverseProxy(healthyBackend),
			}

			unhealthyBackend, _ := url.ParseRequestURI(unavailableServer.URL)
			unhealthyConnection := &connection{
				backend:  unavailableServer.URL,
				healthy:  false,
				messages: make(chan bool),
				proxy:    httputil.NewSingleHostReverseProxy(unhealthyBackend),
			}

			config := &Config{
				Backends: []string{},
				NumConns: 10,
			}
			connectionPool := New(config)
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
	})
}
