// Command provider runs the mock SMS and email Provider.
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

	"campaigncomposer/internal/provider"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	opts, err := provider.OptionsFromEnv(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: addr, Handler: provider.New(opts), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("mock provider listening on %s (429 %.0f%%, 5xx %.0f%%, slow %.0f%% after %v, latency %v-%v)",
		addr, opts.Rate429*100, opts.Rate5xx*100, opts.RateSlow*100, opts.SlowDelay, opts.LatencyMin, opts.LatencyMax)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
