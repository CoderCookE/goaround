package connectionpool

import (
	"net/http"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestHealthCheck(t *testing.T) {
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 1 * time.Second,
	}

	client := &http.Client{Transport: tr}
	assertion := &assert.Asserter{T: t}

	t.Run("backend returns a healthy state", func(t *testing.T) {
		conn := newConnection("", client)

		assertion.False(conn.healthy)
		conn.messages <- true
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health := conn.healthy
		conn.Unlock()
		assertion.True(health)

		conn.messages <- false
		time.Sleep(200 * time.Millisecond)

		conn.Lock()
		health = conn.healthy
		conn.Unlock()

		assertion.False(health)
	})
}
