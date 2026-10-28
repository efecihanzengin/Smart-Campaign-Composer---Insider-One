// Package llm is the seam between the backend and Claude. The real Anthropic
// client and the scripted mock both implement LLM; the backend validates
// whatever comes back and never trusts it.
package llm

import (
	"context"
	"encoding/json"
)

// LLM is what the backend asks of Claude.
type LLM interface {
	// Segment turns a Brief into a Segment Rule proposal: the raw tool input
	// {"rule": …, "unsupported_criteria": […]}, not yet validated.
	Segment(ctx context.Context, req SegmentRequest) (json.RawMessage, error)

	// Content streams the Content tool call's input JSON as it is generated:
	// emit is called with each chunk of partial JSON, and the concatenation of
	// all chunks is the full tool input (see ContentTool), not yet validated.
	// It returns once the call has finished.
	Content(ctx context.Context, req ContentRequest, emit func(chunk string)) error
}

// ContentRequest is the input of the Content call.
type ContentRequest struct {
	Brief string
	// AudienceLabels describe the Segment Rule in readable Turkish, so the copy
	// fits the people it is written for.
	AudienceLabels []string
}

// ContentInstructions is the system prompt of the Content call. It states the
// rules the backend validates, so a well-behaved model rarely needs repair.
const ContentInstructions = `You write marketing Content for one Campaign, through the write_content tool.

- Write in the language of the Brief: a Turkish Brief gets Turkish copy.
- First decide which Channels the Brief asks for and list them in "channels" ("sms", "email"). If the Brief does not restrict the Channels, use both. Write Content only for those Channels.
- SMS: one "body" that fits in a single GSM-7 segment (160 septets). Do not use ş, ğ, ı, İ, Ş, Ğ: transliterate them to s, g, i, I, S, G.
- Email: "subject" (at most 80 characters), "preheader" (at most 120), "body" as plain paragraphs separated by a blank line (at most 1500 characters, no HTML, no Markdown), and an optional short "cta_text" for the button. Never write HTML; the backend renders a fixed template.
- The only placeholders allowed are {{first_name}} and {{city}}. Use no other {{…}}.`

// ContentTool is the input schema of the write_content tool. "channels" comes
// first so the Channel choice streams before the copy.
const ContentTool = `{
  "type": "object",
  "properties": {
    "channels": {"type": "array", "items": {"enum": ["sms", "email"]}},
    "sms": {"type": "object", "properties": {"body": {"type": "string"}}, "required": ["body"]},
    "email": {
      "type": "object",
      "properties": {
        "subject": {"type": "string"},
        "preheader": {"type": "string"},
        "body": {"type": "string"},
        "cta_text": {"type": "string"}
      },
      "required": ["subject", "preheader", "body"]
    }
  },
  "required": ["channels"]
}`

// SegmentRequest is the input of the Segment call.
type SegmentRequest struct {
	Brief string
}
