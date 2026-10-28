package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"campaigncomposer/internal/llm"

	"github.com/anthropics/anthropic-sdk-go/option"
)

const testKey = "sk-ant-test-key"

// fakeAnthropic is a stand-in for the Messages API: it records each request
// body and answers with the response the test scripted.
type fakeAnthropic struct {
	t        *testing.T
	requests []map[string]any
	headers  []http.Header
	respond  func(w http.ResponseWriter)
}

func (f *fakeAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/messages" {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		f.t.Errorf("request body is not JSON: %v", err)
	}
	f.requests = append(f.requests, req)
	f.headers = append(f.headers, r.Header.Clone())
	f.respond(w)
}

func newClaude(t *testing.T, respond func(w http.ResponseWriter)) (*llm.Claude, *fakeAnthropic) {
	t.Helper()
	fake := &fakeAnthropic{t: t, respond: respond}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c := llm.NewClaude(testKey, "claude-test-model", option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	return c, fake
}

func jsonResponse(body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}
}

// toolNames lists the names of the tools a request offered.
func toolNames(req map[string]any) []string {
	var names []string
	tools, _ := req["tools"].([]any)
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

func userText(req map[string]any) string {
	b, _ := json.Marshal(req["messages"])
	var msgs []struct {
		Role    string
		Content []struct{ Type, Text string }
	}
	json.Unmarshal(b, &msgs)
	var sb strings.Builder
	for _, m := range msgs {
		for _, c := range m.Content {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

func TestSegmentReturnsTheProposeSegmentToolInput(t *testing.T) {
	c, fake := newClaude(t, jsonResponse(`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-test-model",
		"content": [
			{"type": "text", "text": "İşte segment:"},
			{"type": "tool_use", "id": "toolu_1", "name": "propose_segment",
			 "input": {"rule": {"all": [{"field": "city", "op": "eq", "value": "İstanbul"}]}, "unsupported_criteria": []}}
		],
		"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 20}
	}`))

	raw, err := c.Segment(context.Background(), llm.SegmentRequest{Brief: "İstanbul'daki kullanıcılara SMS gönder"})
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Segment returned invalid JSON %s: %v", raw, err)
	}
	want := map[string]any{"rule": map[string]any{"all": []any{map[string]any{"field": "city", "op": "eq", "value": "İstanbul"}}},
		"unsupported_criteria": []any{}}
	if gb, wb := mustJSON(got), mustJSON(want); gb != wb {
		t.Errorf("Segment = %s, want %s", gb, wb)
	}

	req := fake.requests[0]
	if req["model"] != "claude-test-model" {
		t.Errorf("model = %v, want claude-test-model", req["model"])
	}
	if names := toolNames(req); len(names) != 1 || names[0] != "propose_segment" {
		t.Errorf("tools = %v, want [propose_segment]", names)
	}
	if !strings.Contains(userText(req), "İstanbul'daki kullanıcılara SMS gönder") {
		t.Errorf("the Brief is not in the user message: %q", userText(req))
	}
	if got := fake.headers[0].Get("X-Api-Key"); got != testKey {
		t.Errorf("x-api-key = %q, want the configured key", got)
	}
}

// sseResponse streams Messages API events, flushing each one.
func sseResponse(events ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			var typ struct{ Type string }
			json.Unmarshal([]byte(e), &typ)
			io.WriteString(w, "event: "+typ.Type+"\ndata: "+e+"\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func contentStream(stopReason string, partials ...string) []string {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test-model","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}`,
		// A thinking block and a text block before the tool call must not leak into the Content.
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"düşünüyorum"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Yazıyorum."}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"write_content","input":{}}}`,
	}
	for _, p := range partials {
		events = append(events, `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":`+mustJSON(p)+`}}`)
	}
	return append(events,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"`+stopReason+`","stop_sequence":null},"usage":{"output_tokens":50}}`,
		`{"type":"message_stop"}`,
	)
}

func TestContentForwardsEachPartialJSONChunkOfTheToolCall(t *testing.T) {
	partials := []string{`{"channels": ["sms"`, `], "sms": {"body": "Merhaba {{first`, `_name}}"}}`}
	c, fake := newClaude(t, sseResponse(contentStream("tool_use", partials...)...))

	var got []string
	err := c.Content(context.Background(), llm.ContentRequest{Brief: "Sepeti terk edenlere SMS", AudienceLabels: []string{"Şehir: İstanbul"}},
		func(chunk string) { got = append(got, chunk) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != strings.Join(partials, "|") {
		t.Errorf("emitted %q, want %q", got, partials)
	}

	req := fake.requests[0]
	if req["stream"] != true {
		t.Errorf("stream = %v, want true", req["stream"])
	}
	if req["model"] != "claude-test-model" {
		t.Errorf("model = %v, want claude-test-model", req["model"])
	}
	tools, _ := req["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "write_content" || tools[0].(map[string]any)["eager_input_streaming"] != true {
		t.Errorf("tools = %v, want write_content with eager_input_streaming", tools)
	}
	text := userText(req)
	if !strings.Contains(text, "Sepeti terk edenlere SMS") || !strings.Contains(text, "Şehir: İstanbul") {
		t.Errorf("user message %q lacks the Brief or the Audience labels", text)
	}
}

func TestRepairCallsSendTheRejectedOutputAndTheValidatorErrors(t *testing.T) {
	repair := &llm.Repair{
		Previous: `{"channels": ["sms"], "sms": {"body": "aaaa"}}`,
		Errors:   []string{"sms.body: SMS tek parçaya sığmalı: GSM-7 ile en fazla 160 karakter olabilir (şu an 172)."},
	}
	c, fake := newClaude(t, sseResponse(contentStream("tool_use", `{"channels": ["sms"], "sms": {"body": "a"}}`)...))
	if err := c.Content(context.Background(), llm.ContentRequest{Brief: "Sepet SMS", Repair: repair}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	seg, segFake := newClaude(t, jsonResponse(`{"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": "propose_segment", "input": {"rule": {"all": []}, "unsupported_criteria": []}}],
		"stop_reason": "tool_use", "usage": {"input_tokens": 1, "output_tokens": 1}}`))
	segRepair := &llm.Repair{Previous: `{"rule": {"all": [`, Errors: []string{"proposal: must be an object: unexpected EOF"}}
	if _, err := seg.Segment(context.Background(), llm.SegmentRequest{Brief: "Sepet SMS", Repair: segRepair}); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		text   string
		repair *llm.Repair
	}{
		"content": {userText(fake.requests[0]), repair},
		"segment": {userText(segFake.requests[0]), segRepair},
	} {
		if !strings.Contains(tc.text, "Sepet SMS") || !strings.Contains(tc.text, tc.repair.Previous) ||
			!strings.Contains(tc.text, tc.repair.Errors[0]) {
			t.Errorf("%s repair message %q lacks the Brief, the rejected output or the errors", name, tc.text)
		}
	}
}

func TestContentFailsWhenTheOutputIsCutOff(t *testing.T) {
	c, _ := newClaude(t, sseResponse(contentStream("max_tokens", `{"channels": ["sms"], "sms": {"bo`)...))
	err := c.Content(context.Background(), llm.ContentRequest{Brief: "b"}, func(string) {})
	if err == nil {
		t.Fatal("Content succeeded on a stream cut off by max_tokens")
	}
}

func TestContentFailsWhenClaudeDoesNotCallTheTool(t *testing.T) {
	c, _ := newClaude(t, sseResponse(
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Üzgünüm."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	))
	var emitted int
	err := c.Content(context.Background(), llm.ContentRequest{Brief: "b"}, func(string) { emitted++ })
	if err == nil || emitted != 0 {
		t.Fatalf("Content = %v after %d chunks, want an error and no chunks", err, emitted)
	}
}

func TestSegmentFailsWhenClaudeDoesNotCallTheTool(t *testing.T) {
	c, _ := newClaude(t, jsonResponse(`{"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
		"content": [{"type": "text", "text": "Anlamadım."}], "stop_reason": "end_turn", "usage": {"input_tokens": 1, "output_tokens": 1}}`))
	if _, err := c.Segment(context.Background(), llm.SegmentRequest{Brief: "b"}); err == nil {
		t.Fatal("Segment succeeded without a propose_segment call")
	}
}

func TestAPIErrorsNeverCarryTheKey(t *testing.T) {
	c, _ := newClaude(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	})
	_, err := c.Segment(context.Background(), llm.SegmentRequest{Brief: "b"})
	if err == nil {
		t.Fatal("Segment succeeded on a 401")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error %q contains the API key", err)
	}
	err = c.Content(context.Background(), llm.ContentRequest{Brief: "b"}, func(string) {})
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("Content error = %v, want an error without the key", err)
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
