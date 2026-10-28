package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"
)

type segmentView struct {
	Rule                segment.Node `json:"rule"`
	Labels              []string     `json:"labels"`
	UnsupportedCriteria []string     `json:"unsupported_criteria"`
}

type campaignView struct {
	ID       int64            `json:"id"`
	Status   string           `json:"status"`
	Brief    string           `json:"brief"`
	Segment  segmentView      `json:"segment"`
	Audience segment.Audience `json:"audience"`
	// EnabledChannels starts as every Channel whose Audience is not empty;
	// DisabledChannels explains each one that was turned off for that reason.
	EnabledChannels  []string          `json:"enabled_channels"`
	DisabledChannels map[string]string `json:"disabled_channels"`
}

// campaignAudience counts a stored Campaign's Audience per Channel.
func campaignAudience(ctx context.Context, q segment.Querier, id int64) (segment.Audience, error) {
	var ruleJSON []byte
	if err := q.QueryRow(ctx, `SELECT segment_rule FROM campaigns WHERE id = $1`, id).Scan(&ruleJSON); err != nil {
		return segment.Audience{}, err
	}
	rule, err := storedRule(ruleJSON)
	if err != nil {
		return segment.Audience{}, err
	}
	return segment.CountAudience(ctx, q, rule)
}

func audienceSize(a segment.Audience, channel string) int {
	if channel == "sms" {
		return a.SMS
	}
	return a.Email
}

// withAudience keeps the Channels whose Audience has a consenting Recipient.
func withAudience(channels []string, a segment.Audience) []string {
	out := []string{}
	for _, ch := range channels {
		if audienceSize(a, ch) > 0 {
			out = append(out, ch)
		}
	}
	return out
}

// disabledChannels explains, per Channel with an empty Audience, why it is
// turned off and can't be turned on.
func disabledChannels(a segment.Audience) map[string]string {
	out := map[string]string{}
	for _, ch := range content.Channels {
		if audienceSize(a, ch) == 0 {
			out[ch] = "Bu hedef kitlede " + channelNames[ch] + " için izin vermiş alıcı yok, bu yüzden kanal kapatıldı."
		}
	}
	return out
}

// createCampaign creates a draft Campaign from a Brief, asks the LLM for its
// Segment Rule and answers with the rule, its labels and the per-Channel
// Audience counts.
func (s *server) createCampaign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Brief string `json:"brief"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return
	}
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Kampanya talebi boş olamaz."})
		return
	}
	ctx := r.Context()

	raw, err := s.opts.LLM.Segment(ctx, llm.SegmentRequest{Brief: brief})
	if err != nil {
		writeError(w, err)
		return
	}
	proposal, err := segment.ParseProposal(raw)
	if err != nil {
		// The repair loop (ADR-0005) will feed this back to Claude; until then the
		// Marketer sees a retryable error.
		log.Printf("invalid Segment Rule: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Model geçersiz bir hedef kitle kuralı üretti. Lütfen tekrar deneyin."})
		return
	}

	ruleJSON, err := json.Marshal(proposal.Rule)
	if err != nil {
		writeError(w, err)
		return
	}
	c := campaignView{Brief: brief}
	if c.Audience, err = segment.CountAudience(ctx, s.pool, proposal.Rule); err != nil {
		writeError(w, fmt.Errorf("count audience: %w", err))
		return
	}
	c.EnabledChannels = withAudience(content.Channels, c.Audience)
	c.DisabledChannels = disabledChannels(c.Audience)
	// The Campaign is stored only once it has a valid Segment Rule, so failed
	// generations leave nothing behind.
	if err := s.pool.QueryRow(ctx, `INSERT INTO campaigns (brief, segment_rule, unsupported_criteria, enabled_channels)
		VALUES ($1, $2, $3, $4) RETURNING id, status`, brief, ruleJSON, proposal.UnsupportedCriteria, c.EnabledChannels).
		Scan(&c.ID, &c.Status); err != nil {
		writeError(w, err)
		return
	}
	c.Segment = segmentView{
		Rule:                proposal.Rule,
		Labels:              segment.Labels(proposal.Rule),
		UnsupportedCriteria: proposal.UnsupportedCriteria,
	}
	writeJSON(w, http.StatusCreated, c)
}
