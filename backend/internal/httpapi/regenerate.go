package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"
)

// regenerationTarget reads the Campaign and the Marketer's optional
// instruction of a regeneration request, writing the error response itself
// when it can't. Only a draft Campaign can be regenerated.
func (s *server) regenerationTarget(w http.ResponseWriter, r *http.Request) (storedCampaign, string, bool) {
	id, ok := campaignID(w, r)
	if !ok {
		return storedCampaign{}, "", false
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek."})
		return storedCampaign{}, "", false
	}
	c, err := s.loadCampaign(r.Context(), id)
	if errors.Is(err, errCampaignNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
		return c, "", false
	}
	if err != nil {
		writeError(w, err)
		return c, "", false
	}
	if c.status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": notDraftRegeneration})
		return c, "", false
	}
	return c, req.Instruction, true
}

const notDraftRegeneration = "Yalnızca taslak kampanya yeniden üretilebilir."

// regenerateContent asks Claude to rewrite one Channel's Content, with the
// Marketer's optional instruction, and streams it over SSE like the first
// Content call. Only that Channel's Content is replaced; the other Channel's,
// edits included, and the enabled Channels stay as they are. content.done
// carries the Campaign's whole Content as stored, with its validation.
func (s *server) regenerateContent(w http.ResponseWriter, r *http.Request) {
	channel := r.PathValue("channel")
	if !slices.Contains(content.Channels, channel) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya ya da kanal bulunamadı."})
		return
	}
	c, instruction, ok := s.regenerationTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	endGeneration, ok := s.trackGeneration(w, r, c.id, notDraftRegeneration)
	if !ok {
		return
	}
	defer endGeneration()
	stored, err := readContent(ctx, s.pool, c.id)
	if err != nil {
		writeError(w, err)
		return
	}
	current, err := stored.channelJSON(channel)
	if err != nil {
		writeError(w, err)
		return
	}
	longest, err := longestValues(ctx, s.pool, c.rule)
	if err != nil {
		writeError(w, err)
		return
	}

	sse, ok := startSSE(w)
	if !ok {
		writeError(w, errors.New("response writer cannot flush"))
		return
	}
	req := llm.ContentRequest{
		Brief:          c.brief,
		AudienceLabels: segment.Labels(c.rule),
		Regenerate:     &llm.Regeneration{Instruction: instruction, Current: current, Channel: channel},
	}
	proposal, _, repairs, err := s.proposeContent(ctx, req, longest,
		func(p content.Proposal) {
			// Only the regenerated Channel streams; the other one is not touched.
			if p, err := onlyChannel(p, channel); err == nil {
				sse.send("content.delta", p)
			}
		},
		func(n repairNotice) { sse.send("content.repair", n) })
	if err != nil {
		log.Printf("regenerate %s content: %v", channel, err)
		sse.send("error", sseError{Error: contentFailed, Retryable: true})
		return
	}
	_, tag, err := s.upsertContent(ctx, c.id, proposal)
	if err == nil && tag.RowsAffected() == 0 {
		err = errors.New("campaign is no longer a draft")
	}
	if err != nil {
		log.Printf("save regenerated %s content: %v", channel, err)
		sse.send("error", sseError{Error: "İçerik kaydedilemedi.", Retryable: false})
		return
	}
	done, err := s.contentState(ctx, c.id, longest)
	if err != nil {
		log.Printf("read content: %v", err)
		sse.send("error", sseError{Error: "İçerik okunamadı. Lütfen sayfayı yenileyin.", Retryable: false})
		return
	}
	done.Repairs = repairs
	sse.send("content.done", done)
}

// regeneratedSegment answers a Segment Rule regeneration: the Campaign with
// its new Segment Rule and Audience, and its existing Content re-validated
// against the new Audience.
type regeneratedSegment struct {
	campaignView
	Content    contentView                   `json:"content"`
	Validation map[string]content.Validation `json:"validation"`
}

// regenerateSegment asks Claude for a new Segment Rule, with the Marketer's
// optional instruction, through the same repair loop as the first one, and
// streams it over SSE like the first one: segment.repair notices, then
// segment.done with the Campaign, or an error event. The
// Content is never regenerated: it is kept as it is, the Marketer's edits
// included, and only validated again against the new Audience's longest
// placeholder values, so Content that no longer fits comes back flagged. A
// Channel whose new Audience is empty is turned off, as on creation.
func (s *server) regenerateSegment(w http.ResponseWriter, r *http.Request) {
	c, instruction, ok := s.regenerationTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	endGeneration, ok := s.trackGeneration(w, r, c.id, notDraftRegeneration)
	if !ok {
		return
	}
	defer endGeneration()
	var unsupported []string
	if err := s.pool.QueryRow(ctx, `SELECT unsupported_criteria FROM campaigns WHERE id = $1`, c.id).
		Scan(&unsupported); err != nil {
		writeError(w, err)
		return
	}
	if unsupported == nil {
		unsupported = []string{}
	}
	current, err := json.Marshal(map[string]any{"rule": c.rule, "unsupported_criteria": unsupported})
	if err != nil {
		writeError(w, err)
		return
	}
	sse, ok := startSSE(w)
	if !ok {
		writeError(w, errors.New("response writer cannot flush"))
		return
	}

	proposal, repairs, err := s.proposeSegment(ctx, llm.SegmentRequest{
		Brief:      c.brief,
		Regenerate: &llm.Regeneration{Instruction: instruction, Current: string(current)},
	}, func(n repairNotice) { sse.send("segment.repair", n) })
	if err != nil {
		sse.fail("regenerate Segment Rule", err, segmentFailed, true)
		return
	}
	const saveFailed = "Hedef kitle kaydedilemedi. Lütfen tekrar deneyin."
	ruleJSON, err := json.Marshal(proposal.Rule)
	if err != nil {
		sse.fail("encode Segment Rule", err, saveFailed, true)
		return
	}
	out := regeneratedSegment{campaignView: campaignView{
		ID: c.id, Status: c.status, Brief: c.brief, Repairs: repairs, Segment: newSegmentView(proposal),
		MockBriefUnrecognised: s.mockBriefUnrecognised(c.brief),
	}}
	if out.Audience, err = segment.CountAudience(ctx, s.pool, proposal.Rule); err != nil {
		sse.fail("count audience", err, saveFailed, true)
		return
	}
	out.DisabledChannels = disabledChannels(out.Audience)
	// Channels stay as the Marketer set them, except those left without a
	// consenting Recipient.
	tag, err := s.pool.Exec(ctx, `UPDATE campaigns SET segment_rule = $2, unsupported_criteria = $3,
			enabled_channels = ARRAY(SELECT ch FROM unnest(enabled_channels) AS ch WHERE ch = ANY ($4))
		WHERE id = $1 AND status = 'draft'`,
		c.id, ruleJSON, proposal.UnsupportedCriteria, withAudience(content.Channels, out.Audience))
	if err == nil && tag.RowsAffected() == 0 {
		sse.fail("regenerate Segment Rule", errors.New("campaign is no longer a draft"),
			notDraftRegeneration, false)
		return
	}
	if err != nil {
		sse.fail("save Segment Rule", err, saveFailed, true)
		return
	}
	longest, err := longestValues(ctx, s.pool, proposal.Rule)
	if err != nil {
		sse.fail("longest placeholder values", err, "İçerik doğrulanamadı. Lütfen sayfayı yenileyin.", false)
		return
	}
	state, err := s.contentState(ctx, c.id, longest)
	if err != nil {
		sse.fail("read content", err, "İçerik okunamadı. Lütfen sayfayı yenileyin.", false)
		return
	}
	out.EnabledChannels, out.Content, out.Validation = state.EnabledChannels, state.Content, state.Validation
	sse.send("segment.done", out)
}

// channelJSON is one Channel's Content as JSON, or "" if it has none.
func (v contentView) channelJSON(channel string) (string, error) {
	var c any
	switch {
	case channel == "sms" && v.SMS != nil:
		c = v.SMS
	case channel == "email" && v.Email != nil:
		c = v.Email
	default:
		return "", nil
	}
	b, err := json.Marshal(c)
	return string(b), err
}

// contentState is the Campaign's stored Content, validated against the
// longest placeholder values of its Audience.
func (s *server) contentState(ctx context.Context, id int64, longest content.LongestValues) (contentDone, error) {
	var enabled []string
	if err := s.pool.QueryRow(ctx, `SELECT enabled_channels FROM campaigns WHERE id = $1`, id).Scan(&enabled); err != nil {
		return contentDone{}, err
	}
	v, err := readContent(ctx, s.pool, id)
	if err != nil {
		return contentDone{}, err
	}
	return contentDone{EnabledChannels: enabled, Content: v, Validation: content.Validate(v.proposal(v.channels()), longest)}, nil
}
