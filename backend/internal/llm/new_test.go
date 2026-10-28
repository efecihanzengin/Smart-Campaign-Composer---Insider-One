package llm_test

import (
	"testing"

	"campaigncomposer/internal/llm"
)

func TestNewUsesTheMockWithoutAKey(t *testing.T) {
	if _, ok := llm.New("", "claude-sonnet-5-5").(*llm.Mock); !ok {
		t.Error("New without a key did not return the mock")
	}
}

func TestNewUsesClaudeWithAKey(t *testing.T) {
	c, ok := llm.New("sk-ant-x", "claude-other").(*llm.Claude)
	if !ok {
		t.Fatal("New with a key did not return the Claude client")
	}
	if c.Model() != "claude-other" {
		t.Errorf("model = %q, want claude-other", c.Model())
	}
}
