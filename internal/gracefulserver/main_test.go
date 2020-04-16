package gracefulserver

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/CoderCookE/goaround/internal/assert"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testHandler(rw http.ResponseWriter, req *http.Request) {
	rw.Write([]byte("ok"))
}

func TestListenAndServe(t *testing.T) {
	assertion := &assert.Asserter{T: t}

	t.Run("gracefully shutsdown", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer func() {
			log.SetOutput(os.Stderr)
		}()

		done := make(chan struct{})
		server := New(&http.Server{
			Addr:        ":9999",
			Handler:     http.HandlerFunc(testHandler),
			ReadTimeout: 1 * time.Second,
		})

		go func() {
			err := server.ListenAndServe()
			assertion.Equal(err, nil)

			<-time.After(500 * time.Millisecond)

			close(done)
		}()

		<-time.After(500 * time.Millisecond)
		process, err := os.FindProcess(os.Getpid())
		assertion.Equal(err, nil)

		err = process.Signal(syscall.SIGTERM)
		assertion.Equal(err, nil)
		<-done

		assertion.StringContains(buf.String(), "Graceful shutdown complete")
	})
}
