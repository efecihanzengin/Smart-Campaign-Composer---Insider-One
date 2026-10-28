package httpapi

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// generationTimeout is how long a started generation call blocks approval. A
// mark older than this is stale: the process running the call died.
const generationTimeout = 5 * time.Minute

var errNotDraft = errors.New("campaign is not a draft")

// beginGeneration marks a generation call for a draft Campaign as running, so
// approval is refused until it ends, and returns the function that ends it.
// The mark is set only while the Campaign is a draft, under the row lock
// approval takes, so a call can't start on an approved Campaign either.
func (s *server) beginGeneration(ctx context.Context, id int64) (end func(), err error) {
	var started time.Time
	err = s.pool.QueryRow(ctx, `UPDATE campaigns SET generation_started_at = clock_timestamp()
		WHERE id = $1 AND status = 'draft' RETURNING generation_started_at`, id).Scan(&started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotDraft
	}
	if err != nil {
		return nil, err
	}
	return func() {
		// Only this call's own mark is cleared: a call started meanwhile keeps
		// blocking approval until it ends too.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET generation_started_at = NULL
			WHERE id = $1 AND generation_started_at = $2`, id, started); err != nil {
			log.Printf("end generation of campaign %d: %v", id, err)
		}
	}, nil
}
