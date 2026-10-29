package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// generationTTL is how long a generation's claim to block approval lasts
// without a heartbeat: three heartbeats, so one slow renewal doesn't drop it.
func (s *server) generationTTL() time.Duration { return 3 * s.opts.GenerationHeartbeat }

// trackGeneration registers a generation call for a draft Campaign, so
// approval is refused while it runs, and returns the function that ends it.
// While the call runs its registration is renewed every heartbeat, so a call
// of any length keeps blocking approval; one whose api process died expires
// within three heartbeats. The registration is added only while the Campaign
// is a draft, under a share lock that waits for an approval holding the row,
// so a call can't start on an approved Campaign either. If it can't register,
// it writes the error response itself (notDraft when the Campaign is no
// longer a draft) and returns false.
func (s *server) trackGeneration(w http.ResponseWriter, r *http.Request, id int64, notDraft string) (end func(), ok bool) {
	ctx := r.Context()
	ttl := s.generationTTL()
	var gen int64
	err := s.pool.QueryRow(ctx, `INSERT INTO generations (campaign_id, expires_at)
		SELECT id, clock_timestamp() + make_interval(secs => $2) FROM campaigns WHERE id = $1 AND status = 'draft' FOR SHARE
		RETURNING id`, id, ttl.Seconds()).Scan(&gen)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": notDraft})
		return nil, false
	}
	if err != nil {
		writeError(w, err)
		return nil, false
	}

	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(s.opts.GenerationHeartbeat)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			if _, err := s.pool.Exec(context.Background(), `UPDATE generations
				SET expires_at = clock_timestamp() + make_interval(secs => $2) WHERE id = $1`, gen, ttl.Seconds()); err != nil {
				log.Printf("renew generation of campaign %d: %v", id, err)
			}
		}
	}()
	return func() {
		close(stop)
		<-stopped
		// Expired registrations of crashed calls go too.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.pool.Exec(ctx, `DELETE FROM generations WHERE id = $1 OR expires_at < now()`, gen); err != nil {
			log.Printf("end generation of campaign %d: %v", id, err)
		}
	}, true
}
