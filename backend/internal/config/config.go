// Package config reads process configuration from the environment.
package config

import (
	"os"
	"strconv"
)

// DefaultModel is the Claude model used when ANTHROPIC_MODEL is not set.
const DefaultModel = "claude-sonnet-5-5"

type Config struct {
	DatabaseURL     string
	HTTPAddr        string
	AnthropicAPIKey string
	AnthropicModel  string
	ProviderURL     string
	// RateLimit is the most Provider requests per second per Channel.
	RateLimit float64
}

func FromEnv() Config {
	return Config{
		DatabaseURL:     env("DATABASE_URL", "postgres://campaign:campaign@localhost:55432/campaign?sslmode=disable"),
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  env("ANTHROPIC_MODEL", DefaultModel),
		ProviderURL:     env("PROVIDER_URL", "http://localhost:8081"),
		RateLimit:       positiveFloat("RATE_LIMIT_PER_SECOND", 100),
	}
}

// MockLLM reports whether the scripted mock LLM stands in for Claude.
func (c Config) MockLLM() bool {
	return c.AnthropicAPIKey == ""
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// positiveFloat reads a positive number, falling back when it is unset or
// not a positive number.
func positiveFloat(key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
