package config_test

import (
	"testing"
	"time"

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

func TestWorkerRetryTuningComesFromTheEnvironment(t *testing.T) {
	t.Setenv("WORKER_BACKOFF_BASE_MS", "250")
	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "1500")
	t.Setenv("WORKER_MAX_ATTEMPTS", "7")
	c := config.FromEnv()
	if c.BackoffBase != 250*time.Millisecond || c.ProviderTimeout != 1500*time.Millisecond || c.MaxAttempts != 7 {
		t.Errorf("backoff %v, timeout %v, max Attempts %d; want 250ms, 1.5s, 7", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
	}
}

func TestUnsetWorkerTuningLeavesTheWorkerDefaults(t *testing.T) {
	t.Setenv("WORKER_BACKOFF_BASE_MS", "")
	t.Setenv("WORKER_PROVIDER_TIMEOUT_MS", "abc")
	t.Setenv("WORKER_MAX_ATTEMPTS", "")
	c := config.FromEnv()
	if c.BackoffBase != 0 || c.ProviderTimeout != 0 || c.MaxAttempts != 0 {
		t.Errorf("backoff %v, timeout %v, max Attempts %d; want zero (worker defaults)", c.BackoffBase, c.ProviderTimeout, c.MaxAttempts)
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
