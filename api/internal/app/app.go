// Package app wires the HTTP surface of the api service.
//
// The service owns the items table and the Redis queue. It never creates
// schema on start: the SQL migrations run through the pre_deploy hook
// (cmd/migrate), so every replica boots against an already migrated database.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Version is returned by /health. Bump it to watch a redeploy land.
const Version = "v1"

// queueName is the Redis list the worker blocks on (BRPOP).
const queueName = "jobs"

// Application holds the two dependencies of the service.
type Application struct {
	Database *pgxpool.Pool
	Queue    *redis.Client
	Secret   string
	Logger   *log.Logger
}

// New builds the application from the environment. DATABASE_URL and REDIS_URL
// are injected by Meshbay as project variables; SIGNING_SECRET is sealed.
func New(logger *log.Logger) (*Application, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, errors.New("REDIS_URL is required")
	}
	connectContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, failure := pgxpool.New(connectContext, databaseURL)
	if failure != nil {
		return nil, fmt.Errorf("connect database: %w", failure)
	}
	if failure := database.Ping(connectContext); failure != nil {
		database.Close()
		return nil, fmt.Errorf("ping database: %w", failure)
	}
	redisOptions, failure := redis.ParseURL(redisURL)
	if failure != nil {
		database.Close()
		return nil, fmt.Errorf("parse REDIS_URL: %w", failure)
	}
	queue := redis.NewClient(redisOptions)
	if failure := queue.Ping(connectContext).Err(); failure != nil {
		database.Close()
		_ = queue.Close()
		return nil, fmt.Errorf("ping redis: %w", failure)
	}
	return &Application{
		Database: database, Queue: queue, Logger: logger,
		Secret: os.Getenv("SIGNING_SECRET"),
	}, nil
}

// Router returns the HTTP handler with CORS, request logging and the
// X-Signature response header.
func (application *Application) Router() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", application.handleHealth)
	router.HandleFunc("GET /api/items", application.handleListItems)
	router.HandleFunc("POST /api/items", application.handleCreateItem)
	router.HandleFunc("GET /api/items/{itemID}", application.handleGetItem)
	router.HandleFunc("PUT /api/items/{itemID}", application.handleUpdateItem)
	router.HandleFunc("DELETE /api/items/{itemID}", application.handleDeleteItem)
	router.HandleFunc("POST /api/items/{itemID}/process", application.handleProcessItem)
	router.HandleFunc("GET /api/stats", application.handleStats)
	return application.withLogging(withCORS(application.withSignature(router)))
}

func (application *Application) Close() {
	if application.Database != nil {
		application.Database.Close()
	}
	if application.Queue != nil {
		_ = application.Queue.Close()
	}
}

func withCORS(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		responseWriter.Header().Set("Access-Control-Allow-Origin", "*")
		responseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		responseWriter.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		responseWriter.Header().Set("Access-Control-Expose-Headers", "X-Signature")
		if httpRequest.Method == http.MethodOptions {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		nextHandler.ServeHTTP(responseWriter, httpRequest)
	})
}

func (application *Application) withLogging(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: responseWriter, status: http.StatusOK}
		nextHandler.ServeHTTP(recorder, httpRequest)
		application.Logger.Printf("%s %s %d %s", httpRequest.Method, httpRequest.URL.Path,
			recorder.status, time.Since(startedAt).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.status = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

type healthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

// handleHealth pings both dependencies. It answers 503 when one of them is
// unreachable so the Meshbay healthcheck fails loudly instead of hiding it.
func (application *Application) handleHealth(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	checkContext, cancel := context.WithTimeout(httpRequest.Context(), 2*time.Second)
	defer cancel()
	payload := healthResponse{Status: "ok", Version: Version, Database: "ok", Redis: "ok"}
	statusCode := http.StatusOK
	if failure := application.Database.Ping(checkContext); failure != nil {
		payload.Status = "degraded"
		payload.Database = failure.Error()
		statusCode = http.StatusServiceUnavailable
	}
	if failure := application.Queue.Ping(checkContext).Err(); failure != nil {
		payload.Status = "degraded"
		payload.Redis = failure.Error()
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(responseWriter, statusCode, payload)
}
