package config_test

import (
	"testing"

	"campaigncomposer/internal/config"
)

func TestModelDefaultsToSonnet(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "")
	if got := config.FromEnv().AnthropicModel; got != "claude-sonnet-5-5" {
		t.Errorf("model = %q, want claude-sonnet-5-5", got)
	}
}

func TestAnthropicModelOverridesTheDefault(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "claude-opus-5-5")
	if got := config.FromEnv().AnthropicModel; got != "claude-opus-5-5" {
		t.Errorf("model = %q, want claude-opus-5-5", got)
	}
}

func TestMockLLMOnlyWithoutAKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if !config.FromEnv().MockLLM() {
		t.Error("MockLLM is false without a key")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-x")
	if config.FromEnv().MockLLM() {
		t.Error("MockLLM is true with a key")
	}
}

func TestRateLimitDefaultsTo100PerSecond(t *testing.T) {
	t.Setenv("RATE_LIMIT_PER_SECOND", "")
	if got := config.FromEnv().RateLimit; got != 100 {
		t.Errorf("rate limit = %v, want 100", got)
	}
}

func TestRateLimitIsConfigurable(t *testing.T) {
	t.Setenv("RATE_LIMIT_PER_SECOND", "20")
	if got := config.FromEnv().RateLimit; got != 20 {
		t.Errorf("rate limit = %v, want 20", got)
	}
}
