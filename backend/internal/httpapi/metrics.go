package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"campaigncomposer/internal/content"
)

// metrics serves per-Channel Delivery counters in the Prometheus text
// format. They are read from the database rather than kept in memory, so
// they cover every worker process and survive restarts: sent and failed
// count Deliveries in those states, retries count Attempts that sent their
// Delivery back to `pending`. All three only grow.
func (s *server) metrics(w http.ResponseWriter, r *http.Request) {
	type counts struct{ sent, failed, retries int64 }
	byChannel := map[string]*counts{}
	for _, ch := range content.Channels {
		byChannel[ch] = &counts{}
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT channel,
			count(*) FILTER (WHERE state = 'sent'),
			count(*) FILTER (WHERE state = 'failed'),
			0
		FROM deliveries GROUP BY channel
		UNION ALL
		SELECT d.channel, 0, 0, count(*)
		FROM attempts a JOIN deliveries d ON d.id = a.delivery_id
		WHERE a.outcome = 'retry' GROUP BY d.channel`)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ch string
		var sent, failed, retries int64
		if err := rows.Scan(&ch, &sent, &failed, &retries); err != nil {
			writeError(w, err)
			return
		}
		c, ok := byChannel[ch]
		if !ok {
			c = &counts{}
			byChannel[ch] = c
		}
		c.sent += sent
		c.failed += failed
		c.retries += retries
	}
	if err := rows.Err(); err != nil {
		writeError(w, err)
		return
	}

	var b strings.Builder
	series := []struct {
		name, help string
		value      func(*counts) int64
	}{
		{"campaign_deliveries_sent_total", "Deliveries the Provider accepted.", func(c *counts) int64 { return c.sent }},
		{"campaign_deliveries_failed_total", "Deliveries given up on: a permanent error or the last allowed Attempt.", func(c *counts) int64 { return c.failed }},
		{"campaign_delivery_retries_total", "Attempts that failed and were scheduled for a retry.", func(c *counts) int64 { return c.retries }},
	}
	for _, m := range series {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n", m.name, m.help, m.name)
		for _, ch := range content.Channels {
			fmt.Fprintf(&b, "%s{channel=%q} %d\n", m.name, ch, m.value(byChannel[ch]))
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
