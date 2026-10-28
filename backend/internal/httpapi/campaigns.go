package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	// Repairs is how many repairs the Segment call needed (ADR-0005).
	Repairs int `json:"repairs"`
}

const segmentFailed = "Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin."

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

	proposal, repairs, err := s.proposeSegment(ctx, brief)
	if errors.Is(err, errRepairsExhausted) {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": segmentFailed, "retryable": true})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}

	ruleJSON, err := json.Marshal(proposal.Rule)
	if err != nil {
		writeError(w, err)
		return
	}
	// The Campaign is stored only once it has a valid Segment Rule, so failed
	// generations leave nothing behind.
	c := campaignView{Brief: brief, Repairs: repairs}
	if err := s.pool.QueryRow(ctx, `INSERT INTO campaigns (brief, segment_rule, unsupported_criteria)
		VALUES ($1, $2, $3) RETURNING id, status`, brief, ruleJSON, proposal.UnsupportedCriteria).
		Scan(&c.ID, &c.Status); err != nil {
		writeError(w, err)
		return
	}
	if c.Audience, err = segment.CountAudience(ctx, s.pool, proposal.Rule); err != nil {
		writeError(w, fmt.Errorf("count audience: %w", err))
		return
	}
	c.Segment = segmentView{
		Rule:                proposal.Rule,
		Labels:              segment.Labels(proposal.Rule),
		UnsupportedCriteria: proposal.UnsupportedCriteria,
	}
	writeJSON(w, http.StatusCreated, c)
}
