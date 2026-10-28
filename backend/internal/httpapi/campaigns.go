package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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
	// The Campaign is stored only once it has a valid Segment Rule, so failed
	// generations leave nothing behind.
	c := campaignView{Brief: brief}
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
