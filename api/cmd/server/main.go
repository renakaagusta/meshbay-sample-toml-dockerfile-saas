// Command server runs the api service: item CRUD, the Redis hand-off to the
// worker and the per-status stats the smoke script reads.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/renakaagusta/meshbay-sample-saas/api/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "api: ", log.LstdFlags|log.LUTC)
	application, failure := app.New(logger)
	if failure != nil {
		logger.Fatalf("start: %v", failure)
	}
	defer application.Close()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           application.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Printf("listening on port %s (version %s)", port, app.Version)
	if application.Secret == "" {
		logger.Printf("SIGNING_SECRET is empty: X-Signature is not sent")
	}
	stopContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveFailure := make(chan error, 1)
	go func() {
		serveFailure <- server.ListenAndServe()
	}()
	select {
	case failure := <-serveFailure:
		if failure != nil && !errors.Is(failure, http.ErrServerClosed) {
			logger.Fatalf("serve: %v", failure)
		}
	case <-stopContext.Done():
		logger.Printf("shutting down")
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if failure := server.Shutdown(shutdownContext); failure != nil {
			logger.Printf("shutdown: %v", failure)
		}
	}
}
