package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/argus-env/argus/internal/database"
	"github.com/argus-env/argus/internal/githubauth"
	"github.com/argus-env/argus/internal/secrets"
	"github.com/argus-env/argus/internal/server"
	"github.com/argus-env/argus/internal/store"
	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

func main() {
	// Load local development configuration without overriding deployment
	// environment variables. godotenv parses assignments without shell execution.
	_ = godotenv.Load()
	logger := log.NewWithOptions(os.Stderr, log.Options{ReportTimestamp: true})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Fatal(err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		logger.Fatal("migrate database", "error", err)
	}
	cipher, err := secrets.New(os.Getenv("ARGUS_ENCRYPTION_KEY"))
	if err != nil {
		logger.Fatal(err)
	}
	github, err := githubauth.New(os.Getenv("GITHUB_CLIENT_ID"))
	if err != nil {
		logger.Fatal(err)
	}
	data := store.New(pool, cipher)

	address := os.Getenv("ARGUS_API_ADDRESS")
	if address == "" {
		address = ":8080"
	}
	httpServer := &http.Server{Addr: address, Handler: server.New(pool, data, github), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		logger.Info("Argus API listening", "address", address)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdown); err != nil {
		logger.Error("shutdown", "error", err)
	}
}
