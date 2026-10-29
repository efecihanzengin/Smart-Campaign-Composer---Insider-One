package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/segment"

	"github.com/jackc/pgx/v5"
)

// approvalResult is a successful approval: the Deliveries created per Channel.
type approvalResult struct {
	ID         int64          `json:"id"`
	Status     string         `json:"status"`
	Deliveries map[string]int `json:"deliveries"`
}

// approveCampaign freezes a draft Campaign's Audience. In one transaction it
// locks the Campaign, re-checks every guard against the data as it is now
// (consent included), renders each Recipient's Content, inserts the
// Deliveries and moves the Campaign to approved.
func (s *server) approveCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := campaignID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var res approvalResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		res, err = approve(ctx, tx, id)
		return err
	})
	var notFound *notFoundError
	var conflict *conflictError
	var blocked *blockedError
	switch {
	case errors.As(err, &notFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": conflict.message})
	case errors.As(err, &blocked):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "Kampanya henüz onaylanamaz.",
			"reasons": blocked.reasons,
		})
	case err != nil:
		writeError(w, err)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

type notFoundError struct{}

func (*notFoundError) Error() string { return "campaign not found" }

type conflictError struct{ message string }

func (e *conflictError) Error() string { return e.message }

// blockReason is one reason approval is refused, shown to the Marketer.
type blockReason struct {
	Code    string `json:"code"`
	Channel string `json:"channel,omitempty"`
	Message string `json:"message"`
}

type blockedError struct{ reasons []blockReason }

func (e *blockedError) Error() string { return fmt.Sprintf("approval blocked: %+v", e.reasons) }

func approve(ctx context.Context, tx pgx.Tx, id int64) (approvalResult, error) {
	var status string
	var ruleJSON []byte
	var enabled []string
	var generating bool
	// The row lock serializes approvals and makes the draft guards of edits,
	// toggles and generation wait for this transaction. A generation counts
	// while its registration is unexpired (see trackGeneration).
	err := tx.QueryRow(ctx, `SELECT status, segment_rule, enabled_channels,
			EXISTS (SELECT 1 FROM generations g WHERE g.campaign_id = c.id AND g.expires_at > clock_timestamp())
		FROM campaigns c WHERE id = $1 FOR UPDATE OF c`, id).
		Scan(&status, &ruleJSON, &enabled, &generating)
	if errors.Is(err, pgx.ErrNoRows) {
		return approvalResult{}, &notFoundError{}
	}
	if err != nil {
		return approvalResult{}, err
	}
	if status != "draft" {
		return approvalResult{}, &conflictError{"Kampanya zaten onaylandı."}
	}
	rule, err := storedRule(ruleJSON)
	if err != nil {
		return approvalResult{}, err
	}
	stored, err := readContent(ctx, tx, id)
	if err != nil {
		return approvalResult{}, err
	}
	p := stored.proposal(enabled)

	// Every guard is checked against the data as it is now: Content is
	// re-validated against the current Audience, and the Audience is read with
	// consent as it stands (ADR-0004).
	// While generation runs the Content isn't final, so that is the only reason.
	if generating {
		return approvalResult{}, &blockedError{[]blockReason{{Code: "generating", Message: "İçerik hâlâ üretiliyor; bitmesini bekleyin."}}}
	}
	var reasons []blockReason
	if len(enabled) == 0 {
		reasons = append(reasons, blockReason{Code: "no_channel", Message: "En az bir kanal açık olmalı."})
	}
	longest, err := longestValues(ctx, tx, rule)
	if err != nil {
		return approvalResult{}, err
	}
	validation := content.Validate(p, longest)
	audience := map[string][]segment.Member{}
	for _, ch := range content.Channels {
		if !slices.Contains(enabled, ch) {
			continue
		}
		if v := validation[ch]; !v.Valid {
			reasons = append(reasons, blockReason{Code: "invalid_content", Channel: ch, Message: invalidMessage(ch, v)})
		}
		if audience[ch], err = segment.Members(ctx, tx, rule, ch); err != nil {
			return approvalResult{}, fmt.Errorf("freeze %s Audience: %w", ch, err)
		}
		if len(audience[ch]) == 0 {
			reasons = append(reasons, blockReason{Code: "empty_audience", Channel: ch, Message: channelNames[ch] +
				" kanalında izin vermiş alıcı yok; kanalı kapatın."})
		}
	}
	if len(reasons) > 0 {
		return approvalResult{}, &blockedError{reasons}
	}

	res := approvalResult{ID: id, Status: "approved", Deliveries: map[string]int{}}
	for ch, members := range audience {
		n, err := insertDeliveries(ctx, tx, id, ch, p, members)
		if err != nil {
			return res, fmt.Errorf("insert %s Deliveries: %w", ch, err)
		}
		res.Deliveries[ch] = n
	}
	_, err = tx.Exec(ctx, `UPDATE campaigns SET status = 'approved', approved_at = now() WHERE id = $1`, id)
	return res, err
}

var channelNames = map[string]string{"sms": "SMS", "email": "E-posta"}

func invalidMessage(channel string, v content.Validation) string {
	issues := make([]string, len(v.Issues))
	for i, is := range v.Issues {
		issues[i] = is.Message
	}
	return channelNames[channel] + " içeriği kurallara uymuyor: " + strings.Join(issues, " ")
}

type smsPayload struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

type emailPayload struct {
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	HTML      string `json:"html"`
}

// insertDeliveries renders the Channel's Content for every member and inserts
// their Deliveries in pending. The unique (Campaign, Recipient, Channel) key
// makes a repeated insert a no-op (ADR-0001). It returns how many were added.
func insertDeliveries(ctx context.Context, tx pgx.Tx, id int64, channel string, p content.Proposal, members []segment.Member) (int, error) {
	ids := make([]int64, len(members))
	payloads := make([]string, len(members))
	for i, m := range members {
		sample := content.Sample{FirstName: m.FirstName, City: m.City}
		var payload any
		switch channel {
		case "sms":
			payload = smsPayload{To: m.Phone, Body: content.RenderSMS(p.SMS.Body, sample)}
		case "email":
			html, err := content.RenderEmailHTML(*p.Email, sample)
			if err != nil {
				return 0, err
			}
			payload = emailPayload{
				To: m.Email, Subject: sample.Render(p.Email.Subject), Preheader: sample.Render(p.Email.Preheader), HTML: html,
			}
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		ids[i], payloads[i] = m.ID, string(b)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO deliveries (campaign_id, recipient_id, channel, payload)
		SELECT $1, r, $2, p FROM unnest($3::bigint[], $4::jsonb[]) AS t(r, p)
		ON CONFLICT (campaign_id, recipient_id, channel) DO NOTHING`, id, channel, ids, payloads)
	return int(tag.RowsAffected()), err
}
