package pool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
	"github.com/CoderCookE/goaround/internal/healthcheck"
)

func waitForHealthCheck(connectionPool *pool, server string) {
	connectionPool.RLock()
	hc := connectionPool.healthChecks[server]
	connectionPool.RUnlock()

	for hc == nil {
		time.Sleep(1 * time.Second)

		connectionPool.RLock()
		hc = connectionPool.healthChecks[server]
		connectionPool.RUnlock()
	}

	hc.Wg.Wait()
	return
}

func TestFetch(t *testing.T) {
	assertion := &assert.Asserter{T: t}
	t.Run("With cache", func(t *testing.T) {
		t.Run("Fetches from cache", func(t *testing.T) {
			var callCount int
			blocker := make(chan bool)

			availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message []byte

				if r.URL.Path == "/health" {
					healthReponse := &healthcheck.Reponse{State: "healthy", Message: ""}
					message, _ = json.Marshal(healthReponse)
				}

				if r.URL.Path == "/foo" {
					callCount += 1
					message = []byte("hello")
				}

				w.Write(message)

				go func() {
					time.Sleep(1 * time.Second)
					blocker <- true
				}()
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

			<-blocker
			for i := 0; i < 5; i++ {
				reader := strings.NewReader("This is a test")
				request := httptest.NewRequest("GET", "http://www.test.com/foo", reader)
				recorder := httptest.NewRecorder()
				connectionPool.Fetch(recorder, request)

				result, err := ioutil.ReadAll(recorder.Result().Body)
				assertion.Equal(err, nil)
				assertion.Equal(recorder.Code, http.StatusOK)
				assertion.Equal(string(result), "hello")
				assertion.LessThan(float64(callCount), float64(5))
			}
		})
	})

	t.Run("No cache", func(t *testing.T) {
		t.Run("fetches each request from server", func(t *testing.T) {
			var callCount int
			blocker := make(chan bool)

			availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message []byte

				if r.URL.Path == "/health" {
					healthReponse := &healthcheck.Reponse{State: "healthy", Message: ""}
					message, _ = json.Marshal(healthReponse)
				}

				if r.URL.Path == "/foo" {
					callCount += 1
					message = []byte("hello")
				}

				w.Write(message)
				go func() {
					time.Sleep(1 * time.Second)
					blocker <- true
				}()
			})

			availableServer := httptest.NewServer(availableHandler)
			defer availableServer.Close()

			backends := []string{availableServer.URL}
			config := &Config{
				Backends: backends,
				NumConns: 1,
			}

			connectionPool := New(config)

			waitForHealthCheck(connectionPool, availableServer.URL)
			<-blocker

			for i := 0; i < 5; i++ {
				reader := strings.NewReader("This is a test")
				request := httptest.NewRequest("GET", "http://www.test.com/foo", reader)
				recorder := httptest.NewRecorder()
				connectionPool.Fetch(recorder, request)

				result, err := ioutil.ReadAll(recorder.Result().Body)
				assertion.Equal(err, nil)
				assertion.Equal(recorder.Code, http.StatusOK)
				assertion.Equal(string(result), "hello")
			}

			assertion.Equal(callCount, 5)
		})

		t.Run("First connection tried is degraded, Uses next connections", func(t *testing.T) {
			callCount := 0
			blocker := make(chan bool)

			availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message []byte

				if r.URL.Path == "/health" {
					healthReponse := &healthcheck.Reponse{State: "healthy", Message: ""}
					message, _ = json.Marshal(healthReponse)
				}

				if r.URL.Path == "/foo" {
					callCount += 1
					message = []byte("hello")
				}

				w.Write(message)
				go func() {
					time.Sleep(1 * time.Second)
					blocker <- true
				}()
			})

			availableServer := httptest.NewServer(availableHandler)
			defer availableServer.Close()

			unavailableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			})

			unavailableServer := httptest.NewServer(unavailableHandler)
			defer unavailableServer.Close()

			config := &Config{
				Backends: []string{unavailableServer.URL, availableServer.URL},
				NumConns: 10,
			}
			connectionPool := New(config)

			waitForHealthCheck(connectionPool, availableServer.URL)
			<-blocker

			reader := strings.NewReader("This is a test")
			request := httptest.NewRequest("GET", "http://www.test.com/foo", reader)
			recorder := httptest.NewRecorder()

			connectionPool.Fetch(recorder, request)

			result, err := ioutil.ReadAll(recorder.Result().Body)
			assertion.Equal(err, nil)

			assertion.Equal(recorder.Code, http.StatusOK)
			assertion.Equal(string(result), `hello`)
		})
	})

	t.Run("Listens on unix socket for updates to backends", func(t *testing.T) {
		blocker := make(chan bool)
		var callCount int

		availableHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var message []byte
			if r.URL.Path == "/health" {
				healthReponse := &healthcheck.Reponse{State: "healthy", Message: ""}
				message, _ = json.Marshal(healthReponse)
			}

			if r.URL.Path == "/foo" {
				callCount += 1
				message = []byte("bar")
			}

			w.Write(message)
			go func() {
				time.Sleep(1 * time.Second)
				blocker <- true
			}()
		})

		availableServer := httptest.NewServer(availableHandler)
		defer availableServer.Close()

		config := &Config{
			Backends: []string{},
			NumConns: 10,
		}

		connectionPool := New(config)
		time.Sleep(1 * time.Second)

		const SockAddr = "/tmp/goaround.sock"
		c, err := net.Dial("unix", SockAddr)
		assertion.Equal(err, nil)
		defer c.Close()

		post := fmt.Sprintf("%s\n", availableServer.URL)
		_, err = c.Write([]byte(post))
		assertion.Equal(err, nil)

		waitForHealthCheck(connectionPool, availableServer.URL)
		<-blocker

		reader := strings.NewReader("This is a test")
		request := httptest.NewRequest("GET", "http://www.test.com/foo", reader)
		recorder := httptest.NewRecorder()

		<-blocker
		connectionPool.Fetch(recorder, request)

		result, err := ioutil.ReadAll(recorder.Result().Body)
		assertion.Equal(err, nil)
		assertion.Equal(recorder.Code, http.StatusOK)
		assertion.Equal(string(result), `bar`)
	})
}
