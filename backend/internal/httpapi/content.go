package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

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

type contentDone struct {
	EnabledChannels []string                      `json:"enabled_channels"`
	Content         contentView                   `json:"content"`
	Validation      map[string]content.Validation `json:"validation"`
}

const contentFailed = "Model geçersiz bir içerik üretti. Lütfen tekrar deneyin."

// generateContent runs the streamed Content call for a draft Campaign and
// forwards it over SSE: content.delta events carry the Content parsed so far,
// and content.done carries the stored Content, the inferred Channels and the
// validation results. A failed call ends with an error event.
func (s *server) generateContent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	ctx := r.Context()
	var brief, status string
	var ruleJSON []byte
	err = s.pool.QueryRow(ctx, `SELECT brief, status, segment_rule FROM campaigns WHERE id = $1`, id).
		Scan(&brief, &status, &ruleJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Yalnızca taslak kampanyanın içeriği üretilebilir."})
		return
	}
	rule, err := storedRule(ruleJSON)
	if err != nil {
		writeError(w, err)
		return
	}
	labels := segment.Labels(rule)
	longest, err := s.longest(ctx, rule)
	if err != nil {
		writeError(w, err)
		return
	}

	sse, ok := startSSE(w)
	if !ok {
		writeError(w, errors.New("response writer cannot flush"))
		return
	}

	var raw strings.Builder
	var last []byte
	err = s.opts.LLM.Content(ctx, llm.ContentRequest{Brief: brief, AudienceLabels: labels}, func(chunk string) {
		raw.WriteString(chunk)
		partial, ok := content.ParsePartial(raw.String())
		if !ok {
			return
		}
		b, err := json.Marshal(partial)
		if err != nil || bytes.Equal(b, last) {
			return
		}
		last = b
		sse.send("content.delta", json.RawMessage(b))
	})
	if err != nil {
		log.Printf("content call: %v", err)
		sse.send("error", map[string]string{"error": contentFailed})
		return
	}
	proposal, err := content.ParseProposal([]byte(raw.String()))
	if err != nil {
		// The repair loop (ADR-0005) will feed this back to Claude.
		log.Printf("invalid Content: %v", err)
		sse.send("error", map[string]string{"error": contentFailed})
		return
	}
	if err := saveContent(ctx, s, id, proposal); err != nil {
		log.Printf("save content: %v", err)
		sse.send("error", map[string]string{"error": "İçerik kaydedilemedi. Lütfen tekrar deneyin."})
		return
	}
	sse.send("content.done", contentDone{
		EnabledChannels: proposal.Channels,
		Content:         contentView{SMS: proposal.SMS, Email: proposal.Email},
		Validation:      content.Validate(proposal, longest),
	})
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
// exactly the Channels Claude inferred.
func saveContent(ctx context.Context, s *server, id int64, p content.Proposal) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
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
		_, err := tx.Exec(ctx, `UPDATE campaigns SET enabled_channels = $2 WHERE id = $1`, id, p.Channels)
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
