package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"

	"github.com/jackc/pgx/v5"
)

const campaignNotFound = "Kampanya bulunamadı."

// campaignID reads the Campaign id from the request path, answering 404
// itself when it is not one.
func campaignID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
		return 0, false
	}
	return id, true
}

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

// longestValues finds the longest placeholder values in each Channel's
// Audience, the values Content lengths are checked with (ADR-0002).
func longestValues(ctx context.Context, q segment.Querier, rule segment.Node) (content.LongestValues, error) {
	sms, email, err := segment.LongestValues(ctx, q, rule)
	if err != nil {
		return content.LongestValues{}, fmt.Errorf("longest placeholder values: %w", err)
	}
	return content.LongestValues{
		SMS:   content.Sample{FirstName: sms.FirstName, City: sms.City},
		Email: content.Sample{FirstName: email.FirstName, City: email.City},
	}, nil
}

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

const segmentFailed = "Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin."

// createCampaign creates a draft Campaign from a Brief and asks the LLM for
// its Segment Rule, streaming over SSE: segment.repair announces each repair
// of a rejected rule, and segment.done carries the Campaign with the rule, its
// labels and the per-Channel Audience counts. A failed call ends with an
// error event.
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
	sse, ok := startSSE(w)
	if !ok {
		writeError(w, errors.New("response writer cannot flush"))
		return
	}

	proposal, repairs, err := s.proposeSegment(ctx, llm.SegmentRequest{Brief: brief},
		func(n repairNotice) { sse.send("segment.repair", n) })
	if err != nil {
		sse.fail("create campaign", err, segmentFailed, true)
		return
	}
	const saveFailed = "Kampanya kaydedilemedi. Lütfen tekrar deneyin."
	ruleJSON, err := json.Marshal(proposal.Rule)
	if err != nil {
		sse.fail("encode Segment Rule", err, saveFailed, true)
		return
	}
	c := campaignView{Brief: brief, Repairs: repairs, Segment: newSegmentView(proposal)}
	if c.Audience, err = segment.CountAudience(ctx, s.pool, proposal.Rule); err != nil {
		sse.fail("count audience", err, saveFailed, true)
		return
	}
	c.EnabledChannels = withAudience(content.Channels, c.Audience)
	c.DisabledChannels = disabledChannels(c.Audience)
	// The Campaign is stored only once it has a valid Segment Rule, so failed
	// generations leave nothing behind.
	if err := s.pool.QueryRow(ctx, `INSERT INTO campaigns (brief, segment_rule, unsupported_criteria, enabled_channels)
		VALUES ($1, $2, $3, $4) RETURNING id, status`, brief, ruleJSON, proposal.UnsupportedCriteria, c.EnabledChannels).
		Scan(&c.ID, &c.Status); err != nil {
		sse.fail("store campaign", err, saveFailed, true)
		return
	}
	sse.send("segment.done", c)
}

func newSegmentView(p segment.Proposal) segmentView {
	return segmentView{Rule: p.Rule, Labels: segment.Labels(p.Rule), UnsupportedCriteria: p.UnsupportedCriteria}
}
