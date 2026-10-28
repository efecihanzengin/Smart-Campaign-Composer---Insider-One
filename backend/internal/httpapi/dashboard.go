package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// channelCounts is a Channel's Deliveries by state. Pending includes those
// in flight (`sending`), since neither has an outcome yet.
type channelCounts struct {
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Total   int `json:"total"`
}

// campaignProgress is a Campaign as the dashboard shows it.
type campaignProgress struct {
	ID         int64                    `json:"id"`
	Brief      string                   `json:"brief"`
	Status     string                   `json:"status"`
	Deliveries map[string]channelCounts `json:"deliveries"`
	// Progress is the share of Deliveries with an outcome, from 0 to 1.
	Progress float64 `json:"progress"`
}

const progressQuery = `
	SELECT c.id, c.brief, c.status, d.channel,
		count(*) FILTER (WHERE d.state IN ('pending', 'sending')),
		count(*) FILTER (WHERE d.state = 'sent'),
		count(*) FILTER (WHERE d.state = 'failed')
	FROM campaigns c
	LEFT JOIN deliveries d ON d.campaign_id = c.id
	WHERE %s
	GROUP BY c.id, d.channel
	ORDER BY c.id DESC, d.channel`

// campaignsProgress reads the Delivery counts of the Campaigns matching where
// (an SQL condition over campaigns c with bound args), newest first.
func campaignsProgress(ctx context.Context, q pgxQuerier, where string, args ...any) ([]campaignProgress, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(progressQuery, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []campaignProgress
	for rows.Next() {
		var id int64
		var brief, status string
		var channel *string
		var c channelCounts
		if err := rows.Scan(&id, &brief, &status, &channel, &c.Pending, &c.Sent, &c.Failed); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != id {
			out = append(out, campaignProgress{ID: id, Brief: brief, Status: status, Deliveries: map[string]channelCounts{}})
		}
		if channel != nil {
			c.Total = c.Pending + c.Sent + c.Failed
			out[len(out)-1].Deliveries[*channel] = c
		}
	}
	for i := range out {
		var done, total int
		for _, c := range out[i].Deliveries {
			done += c.Sent + c.Failed
			total += c.Total
		}
		if total > 0 {
			out[i].Progress = float64(done) / float64(total)
		}
	}
	return out, rows.Err()
}

type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *server) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	cs, err := campaignsProgress(r.Context(), s.pool, "c.id = $1", id)
	switch {
	case err != nil:
		writeError(w, err)
	case len(cs) == 0:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
	default:
		writeJSON(w, http.StatusOK, cs[0])
	}
}

// dashboardLimit is how many of the newest non-draft Campaigns the dashboard shows.
const dashboardLimit = 20

// dashboard streams the per-Channel Delivery counts and progress of the
// newest non-draft Campaigns as `dashboard` events: one when the stream opens
// and another whenever a count or status changes. It runs until the client
// goes away.
func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	sse, ok := startSSE(w)
	if !ok {
		writeError(w, fmt.Errorf("dashboard: streaming unsupported"))
		return
	}
	ctx := r.Context()
	tick := time.NewTicker(s.opts.DashboardInterval)
	defer tick.Stop()
	var last []byte
	for {
		cs, err := campaignsProgress(ctx, s.pool, fmt.Sprintf(
			"c.id IN (SELECT id FROM campaigns WHERE status <> 'draft' ORDER BY id DESC LIMIT %d)", dashboardLimit))
		if err != nil && ctx.Err() == nil {
			log.Printf("dashboard: %v", err)
		}
		if err == nil {
			if cs == nil {
				cs = []campaignProgress{}
			}
			b, _ := json.Marshal(map[string]any{"campaigns": cs})
			if !bytes.Equal(b, last) {
				sse.send("dashboard", json.RawMessage(b))
				last = b
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
