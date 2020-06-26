package gracefulserver

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type GracefulServer struct {
	*http.Server
	errChan      chan error
	shutdownChan chan os.Signal
}

func New(server *http.Server) *GracefulServer {
	gs := &GracefulServer{
		Server:       server,
		errChan:      make(chan error, 1),
		shutdownChan: make(chan os.Signal, 1),
	}

	signal.Notify(gs.shutdownChan, syscall.SIGTERM)

	return gs
}

func (gs *GracefulServer) ListenAndServe() error {
	go func() {
		err := gs.Server.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}

		gs.errChan <- err
	}()

	return gs.listenForSignals()
}

func (gs *GracefulServer) ListenAndServeTLS(certFile, keyFile string) error {
	go func() {
		err := gs.Server.ListenAndServeTLS(certFile, keyFile)
		if err == http.ErrServerClosed {
			err = nil
		}

		gs.errChan <- err
	}()

	return gs.listenForSignals()
}

func (gs *GracefulServer) listenForSignals() error {
	select {
	case err := <-gs.errChan:
		return err
	case <-gs.shutdownChan:
		signal.Stop(gs.shutdownChan)
		log.Printf("Gracefully shutting down")
		defer log.Printf("Graceful shutdown complete")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return gs.Server.Shutdown(ctx)
	}
}
