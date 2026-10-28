package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"campaigncomposer/internal/content"
	"campaigncomposer/internal/llm"
	"campaigncomposer/internal/segment"
)

// errRepairsExhausted means Claude's output was still invalid after
// llm.MaxRepairs repairs; the Marketer gets a retryable error.
var errRepairsExhausted = errors.New("output still invalid after the maximum number of repairs")

// proposeSegment runs the Segment call inside the validate-and-repair loop
// (ADR-0005): a failed call or a rejected Segment Rule is announced through
// onRepair and sent back to Claude with the validator's message, at most
// llm.MaxRepairs times. It returns the valid proposal and how many repairs it
// took.
func (s *server) proposeSegment(ctx context.Context, req llm.SegmentRequest, onRepair func(repairNotice)) (segment.Proposal, int, error) {
	for repairs := 0; ; repairs++ {
		raw, err := s.opts.LLM.Segment(ctx, req)
		if ctx.Err() != nil {
			return segment.Proposal{}, repairs, ctx.Err()
		}
		var proposal segment.Proposal
		if err == nil {
			proposal, err = segment.ParseProposal(raw)
			if err == nil {
				return proposal, repairs, nil
			}
		}
		log.Printf("invalid Segment Rule (after %d of %d repairs): %v", repairs, llm.MaxRepairs, err)
		if repairs == llm.MaxRepairs {
			return segment.Proposal{}, repairs, fmt.Errorf("%w: %v", errRepairsExhausted, err)
		}
		onRepair(repairNotice{Repair: repairs + 1, Max: llm.MaxRepairs, Errors: []string{err.Error()}})
		req.Repair = &llm.Repair{Previous: string(raw), Errors: []string{err.Error()}}
	}
}

// repairNotice tells the UI that a rejected output is being repaired.
type repairNotice struct {
	Repair int      `json:"repair"` // 1-based number of this repair
	Max    int      `json:"max"`
	Errors []string `json:"errors"`
}

// proposeContent runs the streamed Content call inside the validate-and-repair
// loop (ADR-0005). Each attempt's partial Content goes to onDelta as it
// streams; a failed call, malformed JSON or Content the validator rejects is
// announced through onRepair and sent back to Claude, at most llm.MaxRepairs
// times. It returns valid Content, its validation and the repairs it took.
func (s *server) proposeContent(ctx context.Context, req llm.ContentRequest, longest content.LongestValues,
	onDelta func(content.Proposal), onRepair func(repairNotice)) (content.Proposal, map[string]content.Validation, int, error) {
	for repairs := 0; ; repairs++ {
		var raw strings.Builder
		var last []byte
		err := s.opts.LLM.Content(ctx, req, func(chunk string) {
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
			onDelta(partial)
		})
		if ctx.Err() != nil {
			return content.Proposal{}, nil, repairs, ctx.Err()
		}
		var errs []string
		var proposal content.Proposal
		var validation map[string]content.Validation
		if err == nil {
			proposal, err = content.ParseProposal([]byte(raw.String()))
		}
		if err == nil && req.Regenerate != nil && req.Regenerate.Channel != "" {
			// Only the regenerated Channel counts; whatever Claude wrote for
			// the other one is ignored, so it can't trigger a repair either.
			proposal, err = onlyChannel(proposal, req.Regenerate.Channel)
		}
		if err != nil {
			errs = []string{err.Error()}
		} else {
			validation = content.Validate(proposal, longest)
			errs = validationErrors(validation)
		}
		if len(errs) == 0 {
			return proposal, validation, repairs, nil
		}
		log.Printf("invalid Content (after %d of %d repairs): %s", repairs, llm.MaxRepairs, strings.Join(errs, "; "))
		if repairs == llm.MaxRepairs {
			return content.Proposal{}, nil, repairs, fmt.Errorf("%w: %s", errRepairsExhausted, strings.Join(errs, "; "))
		}
		onRepair(repairNotice{Repair: repairs + 1, Max: llm.MaxRepairs, Errors: errs})
		req.Repair = &llm.Repair{Previous: raw.String(), Errors: errs}
	}
}

// validationErrors lists the issues of every Channel as "channel.field: message".
func validationErrors(v map[string]content.Validation) []string {
	var errs []string
	for _, ch := range content.Channels {
		for _, i := range v[ch].Issues {
			errs = append(errs, ch+"."+i.Field+": "+i.Message)
		}
	}
	return errs
}

// onlyChannel narrows a proposal to one Channel's Content.
func onlyChannel(p content.Proposal, channel string) (content.Proposal, error) {
	out := content.Proposal{Channels: []string{channel}}
	switch channel {
	case "sms":
		out.SMS = p.SMS
	case "email":
		out.Email = p.Email
	}
	if out.SMS == nil && out.Email == nil {
		return content.Proposal{}, fmt.Errorf("Content for the %s Channel is missing", channel)
	}
	return out, nil
}
