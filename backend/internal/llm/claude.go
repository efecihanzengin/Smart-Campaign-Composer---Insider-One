package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	segmentToolName = "propose_segment"
	contentToolName = "write_content"
)

// Claude is the real LLM: both calls go to the Anthropic Messages API.
type Claude struct {
	client anthropic.Client
	model  string
}

// NewClaude returns a Claude client using apiKey and model. The environment is
// not consulted (the caller passes configuration explicitly); opts are applied
// last, so tests can point it at a fake API.
func NewClaude(apiKey, model string, opts ...option.RequestOption) *Claude {
	all := append([]option.RequestOption{option.WithoutEnvironmentDefaults(), option.WithAPIKey(apiKey)}, opts...)
	return &Claude{client: anthropic.NewClient(all...), model: model}
}

// New returns the LLM the app runs on: the real Claude client when apiKey is
// set, the scripted mock otherwise.
func New(apiKey, model string) LLM {
	if apiKey == "" {
		return NewMock()
	}
	return NewClaude(apiKey, model)
}

// Model is the Claude model the client calls.
func (c *Claude) Model() string { return c.model }

func (c *Claude) Segment(ctx context.Context, req SegmentRequest) (json.RawMessage, error) {
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:      c.model,
		MaxTokens:  segmentMaxTokens,
		System:     []anthropic.TextBlockParam{{Text: SegmentInstructions}},
		Tools:      []anthropic.ToolUnionParam{tool(segmentToolName, "Propose the Segment Rule for the Brief.", SegmentTool, false)},
		ToolChoice: autoToolChoice(),
		Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("Brief:\n" + req.Brief + repairText(req.Repair, segmentToolName)))},
	})
	if err != nil {
		return nil, fmt.Errorf("segment call: %w", err)
	}
	if err := checkStop(msg.StopReason); err != nil {
		return nil, fmt.Errorf("segment call: %w", err)
	}
	for _, block := range msg.Content {
		if b, ok := block.AsAny().(anthropic.ToolUseBlock); ok && b.Name == segmentToolName {
			return json.RawMessage(b.JSON.Input.Raw()), nil
		}
	}
	return nil, fmt.Errorf("segment call: Claude did not call %s", segmentToolName)
}

// Content streams the write_content tool call. The tool sets
// eager_input_streaming, so the API forwards input JSON as it is generated
// instead of buffering each value; it then no longer validates the input, which
// is fine because the backend validates everything anyway. Thinking and text
// blocks before the call are skipped.
func (c *Claude) Content(ctx context.Context, req ContentRequest, emit func(chunk string)) error {
	var user strings.Builder
	user.WriteString("Brief:\n" + req.Brief)
	if len(req.AudienceLabels) > 0 {
		user.WriteString("\n\nAudience (the Segment Rule, as shown to the Marketer):")
		for _, l := range req.AudienceLabels {
			user.WriteString("\n- " + l)
		}
	}
	user.WriteString(repairText(req.Repair, contentToolName))
	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:      c.model,
		MaxTokens:  contentMaxTokens,
		System:     []anthropic.TextBlockParam{{Text: ContentInstructions}},
		Tools:      []anthropic.ToolUnionParam{tool(contentToolName, "Write the Campaign's Content for each chosen Channel.", ContentTool, true)},
		ToolChoice: autoToolChoice(),
		Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user.String()))},
	})
	defer stream.Close()

	toolIndex := int64(-1)
	var stop anthropic.StopReason
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			if b, ok := ev.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok && b.Name == contentToolName && toolIndex < 0 {
				toolIndex = ev.Index
			}
		case anthropic.ContentBlockDeltaEvent:
			if d, ok := ev.Delta.AsAny().(anthropic.InputJSONDelta); ok && ev.Index == toolIndex && d.PartialJSON != "" {
				emit(d.PartialJSON)
			}
		case anthropic.MessageDeltaEvent:
			stop = ev.Delta.StopReason
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("content call: %w", err)
	}
	if err := checkStop(stop); err != nil {
		return fmt.Errorf("content call: %w", err)
	}
	if toolIndex < 0 {
		return fmt.Errorf("content call: Claude did not call %s", contentToolName)
	}
	return nil
}

// repairText is appended to the user message of a repair call (ADR-0005): the
// rejected tool input and why the backend rejected it.
func repairText(r *Repair, toolName string) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nYour previous answer was rejected by the backend validator.")
	if r.Previous != "" {
		b.WriteString("\nPrevious " + toolName + " input:\n" + r.Previous)
	}
	b.WriteString("\nErrors:")
	for _, e := range r.Errors {
		b.WriteString("\n- " + e)
	}
	b.WriteString("\nFix every error and call " + toolName + " again with the complete, corrected input.")
	return b.String()
}

const (
	segmentMaxTokens = 16000
	contentMaxTokens = 32000
)

// tool builds a tool definition from one of the schema constants.
func tool(name, description, schema string, eager bool) anthropic.ToolUnionParam {
	var s struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		panic(fmt.Sprintf("llm: invalid %s schema: %v", name, err))
	}
	t := anthropic.ToolParam{
		Name:        name,
		Description: anthropic.String(description),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: s.Properties, Required: s.Required},
	}
	if eager {
		t.EagerInputStreaming = anthropic.Bool(true)
	}
	return anthropic.ToolUnionParam{OfTool: &t}
}

// autoToolChoice lets Claude pick the tool itself. Forcing a tool
// (tool_choice "tool" or "any") is rejected with a 400 by current models such
// as claude-sonnet-5-5, so the system prompt asks for the call instead, and a
// response without it is an error the repair loop can retry.
func autoToolChoice() anthropic.ToolChoiceUnionParam {
	return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{DisableParallelToolUse: anthropic.Bool(true)}}
}

// checkStop rejects stop reasons that mean the tool input is not usable.
func checkStop(reason anthropic.StopReason) error {
	switch reason {
	case anthropic.StopReasonMaxTokens:
		return fmt.Errorf("Claude hit the output token limit before finishing")
	case anthropic.StopReasonRefusal:
		return fmt.Errorf("Claude declined the request")
	}
	return nil
}
