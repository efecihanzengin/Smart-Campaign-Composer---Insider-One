package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type attemptJSON struct {
	Number     int       `json:"number"`
	StatusCode *int      `json:"status_code"`
	LatencyMS  int64     `json:"latency_ms"`
	Error      string    `json:"error"`
	Outcome    string    `json:"outcome"`
	At         time.Time `json:"at"`
}

type deliveryJSON struct {
	ID           int64         `json:"id"`
	Channel      string        `json:"channel"`
	State        string        `json:"state"`
	AttemptCount int           `json:"attempt_count"`
	Attempts     []attemptJSON `json:"attempts"`
}

// listDeliveries lists a Campaign's Deliveries with their Attempt log, so an
// operator can see why one was retried or failed. `state` and `channel`
// filter; `limit` (default 50, at most 500) bounds the list.
func (s *server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit 1 ile 500 arasında olmalı."})
			return
		}
		limit = n
	}
	var exists bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1)`, id).Scan(&exists); err != nil {
		writeError(w, err)
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}

	rows, err := s.pool.Query(r.Context(), `
		SELECT d.id, d.channel, d.state, d.attempt_count,
			COALESCE((SELECT json_agg(json_build_object(
					'number', a.number, 'status_code', a.status_code, 'latency_ms', a.latency_ms,
					'error', a.error, 'outcome', a.outcome, 'at', a.started_at) ORDER BY a.number, a.id)
				FROM attempts a WHERE a.delivery_id = d.id), '[]')
		FROM deliveries d
		WHERE d.campaign_id = $1
			AND ($2 = '' OR d.state = $2)
			AND ($3 = '' OR d.channel = $3)
		ORDER BY d.id
		LIMIT $4`, id, q.Get("state"), q.Get("channel"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (deliveryJSON, error) {
		var d deliveryJSON
		var attempts []byte
		if err := row.Scan(&d.ID, &d.Channel, &d.State, &d.AttemptCount, &attempts); err != nil {
			return d, err
		}
		return d, json.Unmarshal(attempts, &d.Attempts)
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}
