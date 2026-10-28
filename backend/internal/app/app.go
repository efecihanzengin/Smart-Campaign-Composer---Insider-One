// Package app holds what the api process does on startup.
package app

import (
	"context"
	"log"
	"time"

	"campaigncomposer/internal/db"
	"campaigncomposer/internal/seed"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Bootstrap brings a database up to date: it applies migrations, then seeds
// Recipients and Events if there are none yet.
func Bootstrap(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	seeded, err := seed.Run(ctx, pool, seed.Anchor(now))
	if err != nil {
		return err
	}
	if seeded {
		log.Print("seeded Recipients and Events")
	}
	return nil
}
