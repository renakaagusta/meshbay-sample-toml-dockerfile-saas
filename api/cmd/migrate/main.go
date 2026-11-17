// Command migrate applies the SQL migrations embedded in the api image.
//
// meshbay.toml runs it as the api service pre_deploy hook:
//
//	pre_deploy = "/app/migrate"
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renakaagusta/meshbay-sample-saas/api/internal/migrate"
)

func main() {
	logger := log.New(os.Stdout, "migrate: ", log.LstdFlags|log.LUTC)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}
	runContext, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, failure := pgxpool.New(runContext, databaseURL)
	if failure != nil {
		logger.Fatalf("connect database: %v", failure)
	}
	defer database.Close()
	if failure := migrate.Run(runContext, database, logger); failure != nil {
		logger.Fatalf("migrate: %v", failure)
	}
}
