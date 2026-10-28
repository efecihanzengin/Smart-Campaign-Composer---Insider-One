package llm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"campaigncomposer/internal/llm"
)

const brief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

func mockContent(t *testing.T, req llm.ContentRequest) map[string]map[string]string {
	t.Helper()
	var raw strings.Builder
	if err := llm.NewMock().Content(context.Background(), req, func(s string) { raw.WriteString(s) }); err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw.String()), &out); err != nil {
		t.Fatalf("mock Content %q: %v", raw.String(), err)
	}
	got := map[string]map[string]string{}
	for k, v := range out {
		if k == "channels" {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(v, &m); err != nil {
			t.Fatal(err)
		}
		got[k] = m
	}
	return got
}

func TestMockRegeneratesOnlyTheAskedChannelAndShortensOnRequest(t *testing.T) {
	first := mockContent(t, llm.ContentRequest{Brief: brief})
	again := mockContent(t, llm.ContentRequest{Brief: brief, Regenerate: &llm.Regeneration{Channel: "sms"}})
	shorter := mockContent(t, llm.ContentRequest{Brief: brief, Regenerate: &llm.Regeneration{Channel: "sms", Instruction: "daha kısa"}})

	if _, ok := again["email"]; ok {
		t.Errorf("regenerated SMS also wrote an email: %v", again)
	}
	if again["sms"]["body"] == first["sms"]["body"] {
		t.Errorf("regenerated SMS = %q, want a different text than the first", again["sms"]["body"])
	}
	if len(shorter["sms"]["body"]) >= len(first["sms"]["body"]) {
		t.Errorf("\"daha kısa\" SMS = %q, want shorter than %q", shorter["sms"]["body"], first["sms"]["body"])
	}
	email := mockContent(t, llm.ContentRequest{Brief: brief, Regenerate: &llm.Regeneration{Channel: "email"}})
	if _, ok := email["sms"]; ok || email["email"]["subject"] == first["email"]["subject"] {
		t.Errorf("regenerated email = %v, want only a different email", email)
	}
}

func TestMockSegmentFollowsTheInstruction(t *testing.T) {
	raw, err := llm.NewMock().Segment(context.Background(), llm.SegmentRequest{
		Brief: brief, Regenerate: &llm.Regeneration{Instruction: "sadece Ankara, sadece kadınlar"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"value":"Ankara"`) || strings.Contains(s, "İstanbul") {
		t.Errorf("segment %s, want Ankara instead of İstanbul", s)
	}
	if !strings.Contains(s, "Cinsiyet") {
		t.Errorf("segment %s, want gender listed as unsupported", s)
	}
}
