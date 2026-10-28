package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"

	"github.com/jackc/pgx/v5"
)

// contentView is a Campaign's Content, one entry per Channel that has Content.
type contentView struct {
	SMS   *content.SMS   `json:"sms"`
	Email *content.Email `json:"email"`
}

// readContent reads a Campaign's stored Content.
func readContent(ctx context.Context, q segment.Querier, id int64) (contentView, error) {
	var v contentView
	rows, err := q.Query(ctx, `SELECT channel, subject, preheader, body, cta_text FROM contents WHERE campaign_id = $1`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel string
		var e content.Email
		if err := rows.Scan(&channel, &e.Subject, &e.Preheader, &e.Body, &e.CTAText); err != nil {
			return v, err
		}
		switch channel {
		case "sms":
			v.SMS = &content.SMS{Body: e.Body}
		case "email":
			v.Email = &e
		}
	}
	return v, rows.Err()
}

// channels lists the Channels that have Content, in display order.
func (v contentView) channels() []string {
	var out []string
	if v.SMS != nil {
		out = append(out, "sms")
	}
	if v.Email != nil {
		out = append(out, "email")
	}
	return out
}

// proposal is the Content of the given Channels, as the validator takes it;
// a listed Channel without Content is validated as missing.
func (v contentView) proposal(channels []string) content.Proposal {
	p := content.Proposal{Channels: channels}
	if slices.Contains(channels, "sms") {
		p.SMS = v.SMS
	}
	if slices.Contains(channels, "email") {
		p.Email = v.Email
	}
	return p
}

type contentDone struct {
	EnabledChannels []string                      `json:"enabled_channels"`
	Content         contentView                   `json:"content"`
	Validation      map[string]content.Validation `json:"validation"`
	// Repairs is how many repairs the Content call needed (ADR-0005).
	Repairs int `json:"repairs"`
}

const contentFailed = "Model geçerli bir içerik üretemedi. Lütfen tekrar deneyin."

// generateContent runs the streamed Content call for a draft Campaign and
// forwards it over SSE: content.delta events carry the Content parsed so far,
// and content.done carries the stored Content, the inferred Channels and the
// validation results. A failed call ends with an error event.
func (s *server) generateContent(w http.ResponseWriter, r *http.Request) {
	id, ok := campaignID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	const notDraft = "Yalnızca taslak kampanyanın içeriği üretilebilir."
	c, err := s.loadCampaign(ctx, id)
	if errors.Is(err, errCampaignNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if c.status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": notDraft})
		return
	}
	longest, err := longestValues(ctx, s.pool, c.rule)
	if err != nil {
		writeError(w, err)
		return
	}
	endGeneration, ok := s.trackGeneration(w, r, id, notDraft)
	if !ok {
		return
	}
	defer endGeneration()

	sse, ok := startSSE(w)
	if !ok {
		writeError(w, errors.New("response writer cannot flush"))
		return
	}

	proposal, validation, repairs, err := s.proposeContent(ctx, llm.ContentRequest{Brief: c.brief, AudienceLabels: segment.Labels(c.rule)}, longest,
		func(p content.Proposal) { sse.send("content.delta", p) },
		func(n repairNotice) { sse.send("content.repair", n) })
	if err != nil {
		// Nothing is stored: invalid output never becomes the proposal.
		log.Printf("content call: %v", err)
		sse.send("error", sseError{Error: contentFailed, Retryable: true})
		return
	}
	enabled, err := s.saveContent(ctx, id, proposal)
	if err != nil {
		log.Printf("save content: %v", err)
		sse.send("error", sseError{Error: "İçerik kaydedilemedi. Lütfen tekrar deneyin.", Retryable: true})
		return
	}
	sse.send("content.done", contentDone{
		EnabledChannels: enabled,
		Content:         contentView{SMS: proposal.SMS, Email: proposal.Email},
		Validation:      validation,
		Repairs:         repairs,
	})
}

// sseError ends a generation stream that failed; Retryable tells the UI to
// offer the Marketer another try.
type sseError struct {
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}

// storedRule decodes a Campaign's stored Segment Rule.
func storedRule(ruleJSON []byte) (segment.Node, error) {
	p, err := segment.ParseProposal(json.RawMessage(fmt.Sprintf(`{"rule": %s, "unsupported_criteria": []}`, ruleJSON)))
	if err != nil {
		return nil, fmt.Errorf("stored Segment Rule: %w", err)
	}
	return p.Rule, nil
}

// saveContent replaces the Campaign's Content with the proposal's and enables
// the Channels Claude inferred, except those whose Audience is empty. It
// returns the enabled Channels.
func (s *server) saveContent(ctx context.Context, id int64, p content.Proposal) (enabled []string, err error) {
	return enabled, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		audience, err := campaignAudience(ctx, tx, id)
		if err != nil {
			return err
		}
		enabled = withAudience(p.Channels, audience)
		if _, err := tx.Exec(ctx, `DELETE FROM contents WHERE campaign_id = $1`, id); err != nil {
			return err
		}
		if p.SMS != nil && slices.Contains(p.Channels, "sms") {
			if _, err := tx.Exec(ctx, `INSERT INTO contents (campaign_id, channel, body) VALUES ($1, 'sms', $2)`,
				id, p.SMS.Body); err != nil {
				return err
			}
		}
		if e := p.Email; e != nil && slices.Contains(p.Channels, "email") {
			if _, err := tx.Exec(ctx, `INSERT INTO contents (campaign_id, channel, subject, preheader, body, cta_text)
				VALUES ($1, 'email', $2, $3, $4, $5)`, id, e.Subject, e.Preheader, e.Body, e.CTAText); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE campaigns SET enabled_channels = $2 WHERE id = $1`, id, enabled)
		return err
	})
}

// setChannel enables or disables one Channel of a draft Campaign.
func (s *server) setChannel(w http.ResponseWriter, r *http.Request) {
	channel := r.PathValue("channel")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !slices.Contains(content.Channels, channel) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya ya da kanal bulunamadı."})
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return
	}
	if *req.Enabled {
		audience, err := campaignAudience(r.Context(), s.pool, id)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Taslak kampanya bulunamadı."})
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		if why, empty := disabledChannels(audience)[channel]; empty {
			writeJSON(w, http.StatusConflict, map[string]string{"error": why})
			return
		}
	}
	// Channels are kept in display order, whatever order they were toggled in.
	var enabled []string
	err = s.pool.QueryRow(r.Context(), `
		UPDATE campaigns SET enabled_channels = ARRAY(
			SELECT ch FROM unnest(ARRAY['sms', 'email']) WITH ORDINALITY AS t(ch, ord)
			WHERE CASE WHEN ch = $2 THEN $3 ELSE ch = ANY (enabled_channels) END
			ORDER BY ord)
		WHERE id = $1 AND status = 'draft'
		RETURNING enabled_channels`, id, channel, *req.Enabled).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Taslak kampanya bulunamadı."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"enabled_channels": enabled})
}

// previewEmail renders email Content into the fixed HTML template with sample
// placeholder values.
func (s *server) previewEmail(w http.ResponseWriter, r *http.Request) {
	var e content.Email
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return
	}
	html, err := content.RenderEmailHTML(e, content.PreviewSample)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func startSSE(w http.ResponseWriter) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sseWriter{w: w, f: f}, true
}

// fail logs why a generation stream failed and ends it with an error event
// carrying the Marketer-facing message.
func (s *sseWriter) fail(what string, err error, message string, retryable bool) {
	log.Printf("%s: %v", what, err)
	s.send("error", sseError{Error: message, Retryable: retryable})
}

// send writes one event; data is JSON-encoded on a single line.
func (s *sseWriter) send(event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		log.Printf("encode %s event: %v", event, err)
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	s.f.Flush()
}
