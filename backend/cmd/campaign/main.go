// Command campaign runs the API (`campaign api`) or the Delivery worker
// (`campaign worker`).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"campaigncomposer/internal/app"
	"campaigncomposer/internal/config"
	"campaigncomposer/internal/db"
	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "api" && os.Args[1] != "worker") {
		fmt.Fprintln(os.Stderr, "usage: campaign api|worker")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.FromEnv()
	var err error
	if os.Args[1] == "api" {
		err = runAPI(ctx, cfg)
	} else {
		err = runWorker(ctx, cfg)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func runAPI(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, 60*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Migrations and the seed finish before the API listens, so a healthy API
	// means a ready database.
	if err := app.Bootstrap(ctx, pool, time.Now()); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// The key itself is never logged; only whether one is set.
	mock := cfg.MockLLM()
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewHandler(pool, httpapi.Options{
			MockLLM: mock,
			Model:   cfg.AnthropicModel,
			LLM:     llm.New(cfg.AnthropicAPIKey, cfg.AnthropicModel),
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("api listening on %s (mock LLM: %t, model: %s)", cfg.HTTPAddr, mock, cfg.AnthropicModel)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// runWorker connects to the database and waits; claiming and sending
// Deliveries arrives with the sending slice.
func runWorker(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, 60*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Printf("worker ready (Provider at %s); no Deliveries to send yet", cfg.ProviderURL)
	<-ctx.Done()
	return nil
}
