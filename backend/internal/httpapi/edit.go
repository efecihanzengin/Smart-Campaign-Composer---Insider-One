package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/segment"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// storedCampaign is what validation and regeneration need of a Campaign.
type storedCampaign struct {
	id     int64
	brief  string
	status string
	rule   segment.Node
}

var errCampaignNotFound = errors.New("campaign not found")

// loadCampaign reads a Campaign's Brief, status and Segment Rule.
func (s *server) loadCampaign(ctx context.Context, id int64) (storedCampaign, error) {
	c := storedCampaign{id: id}
	var ruleJSON []byte
	err := s.pool.QueryRow(ctx, `SELECT brief, status, segment_rule FROM campaigns WHERE id = $1`, id).
		Scan(&c.brief, &c.status, &ruleJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, errCampaignNotFound
	}
	if err != nil {
		return c, err
	}
	c.rule, err = storedRule(ruleJSON)
	return c, err
}

// longest finds the longest placeholder values in each Channel's Audience.
func (s *server) longest(ctx context.Context, rule segment.Node) (content.Longest, error) {
	sms, email, err := segment.LongestValues(ctx, s.pool, rule)
	if err != nil {
		return content.Longest{}, fmt.Errorf("longest placeholder values: %w", err)
	}
	return content.Longest{
		SMS:   content.Sample{FirstName: sms.FirstName, City: sms.City},
		Email: content.Sample{FirstName: email.FirstName, City: email.City},
	}, nil
}

// contentRequest reads a Campaign and a Channel's Content from an edit or
// validate request, writing the error response itself when it can't.
func (s *server) contentRequest(w http.ResponseWriter, r *http.Request) (storedCampaign, content.Proposal, bool) {
	channel := r.PathValue("channel")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !slices.Contains(content.Channels, channel) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya ya da kanal bulunamadı."})
		return storedCampaign{}, content.Proposal{}, false
	}
	p := content.Proposal{Channels: []string{channel}}
	var target any = &p.SMS
	if channel == "email" {
		target = &p.Email
	}
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return storedCampaign{}, content.Proposal{}, false
	}
	c, err := s.loadCampaign(r.Context(), id)
	if errors.Is(err, errCampaignNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return c, p, false
	}
	if err != nil {
		writeError(w, err)
		return c, p, false
	}
	return c, p, true
}

// validate runs the Content validator over one Channel's Content against the
// Campaign's current Audience. The live counters and inline edits use it, so
// the rules exist only in the backend (ADR-0002).
func (s *server) validate(ctx context.Context, c storedCampaign, p content.Proposal) (content.Validation, error) {
	longest, err := s.longest(ctx, c.rule)
	if err != nil {
		return content.Validation{}, err
	}
	return content.Validate(p, longest)[p.Channels[0]], nil
}

// validateContent answers with the validation of unsaved Content.
func (s *server) validateContent(w http.ResponseWriter, r *http.Request) {
	c, p, ok := s.contentRequest(w, r)
	if !ok {
		return
	}
	v, err := s.validate(r.Context(), c, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// updateContent saves the Marketer's edit of one Channel's Content and
// answers with it and its validation. Invalid Content is saved too, so no
// edit is lost; approval rejects it.
func (s *server) updateContent(w http.ResponseWriter, r *http.Request) {
	c, p, ok := s.contentRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	saved, tag, err := s.upsertContent(ctx, c.id, p)
	if errors.Is(err, errNoContent) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Yalnızca taslak kampanyanın içeriği düzenlenebilir."})
		return
	}
	v, err := s.validate(ctx, c, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": saved, "validation": v})
}

var errNoContent = errors.New("no Content to save")

// upsertContent saves the one Channel's Content in p and leaves the other
// Channel's alone. The draft check is repeated in the write, under a share
// lock that waits for an approval holding the row, so a write racing approval
// can't change Content that is already frozen: it affects no row.
func (s *server) upsertContent(ctx context.Context, id int64, p content.Proposal) (any, pgconn.CommandTag, error) {
	if sms := p.SMS; sms != nil {
		tag, err := s.pool.Exec(ctx, `INSERT INTO contents (campaign_id, channel, body)
			SELECT id, 'sms', $2 FROM campaigns WHERE id = $1 AND status = 'draft' FOR SHARE
			ON CONFLICT (campaign_id, channel) DO UPDATE SET body = EXCLUDED.body, updated_at = now()`, id, sms.Body)
		return sms, tag, err
	}
	if e := p.Email; e != nil {
		tag, err := s.pool.Exec(ctx, `INSERT INTO contents (campaign_id, channel, subject, preheader, body, cta_text)
			SELECT id, 'email', $2, $3, $4, $5 FROM campaigns WHERE id = $1 AND status = 'draft' FOR SHARE
			ON CONFLICT (campaign_id, channel) DO UPDATE SET subject = EXCLUDED.subject, preheader = EXCLUDED.preheader,
				body = EXCLUDED.body, cta_text = EXCLUDED.cta_text, updated_at = now()`,
			id, e.Subject, e.Preheader, e.Body, e.CTAText)
		return e, tag, err
	}
	return nil, pgconn.CommandTag{}, errNoContent
}
