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

// SegmentInstructions is the system prompt of the Segment call. It describes
// the whitelisted Segment Rule language (ADR-0003) the backend validates.
const SegmentInstructions = `You turn a Marketer's Brief into a Segment Rule, through the propose_segment tool. Always answer by calling propose_segment exactly once.

A Segment Rule is a JSON tree, at most 3 levels deep, built only from these nodes:
- {"all": [node, ...]}: every child matches. Use it at the top level.
- {"any": [node, ...]}: at least one child matches.
- {"not": node}: the child does not match.
- Profile condition: {"field": "city", "op": "eq", "value": "İstanbul"} or {"field": "city", "op": "in", "value": ["İstanbul", "Ankara"]}; {"field": "language", "op": "eq" | "in", "value": two-letter code such as "tr"}; {"field": "created_at", "op": "before" | "after", "value": "YYYY-MM-DD"}.
- Event condition: {"event": "cart_add" | "purchase" | "page_view", "within_days": 1-365, "has": true | false}, optionally with "min_count" (1-1000) and, for purchase only, "min_amount" (a positive TRY amount as a JSON number).

Rules:
- "Sepete ekledi" is cart_add, "alışveriş yaptı" / "satın aldı" is purchase, "siteyi ziyaret etti" is page_view. "Yapmayan" / "almayan" means "has": false.
- Write city names with their Turkish spelling (İstanbul, İzmir).
- Never include consent: consent is applied separately for every Channel. Ignore which Channel the Brief names; that is decided elsewhere.
- List every part of the Brief's audience that this language cannot express (for example a product category) in "unsupported_criteria", in the language of the Brief. Never silently drop or broaden a criterion. Use an empty list when everything fits.`

// SegmentTool is the input schema of the propose_segment tool.
const SegmentTool = `{
  "type": "object",
  "properties": {
    "rule": {
      "type": "object",
      "description": "The Segment Rule: a tree of all / any / not nodes over profile and Event conditions, as described in the instructions."
    },
    "unsupported_criteria": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Parts of the Brief's audience the Segment Rule could not express."
    }
  },
  "required": ["rule", "unsupported_criteria"]
}`
