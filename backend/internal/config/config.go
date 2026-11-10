// Package config reads service settings from environment variables.
package config

import (
	"fmt"
	"os"
)

// The laamo dev project on Eurobase (not secret: it's in every token).
const (
	DevEurobaseURL = "https://laamo.eurobase.app"
	DevProjectID   = "f852e670-cdc2-4738-856d-be7e1947e2fb"
)

// Config holds settings shared by all Laamo services.
type Config struct {
	Env               string // dev, staging, prod
	APIAddr           string
	EurobaseURL       string
	EurobaseProjectID string // tokens must be issued for this project (aud)
	EurobasePublicKey string
	EurobaseSecretKey string // server side only
	BraveSearchAPIKey string
	// Master keys for secrets at rest ("v1:base64,v2:base64"), see
	// internal/crypto and docs/security/secrets.md. Server side only.
	MasterKeys      string
	MasterKeyActive string
	// The AI brain: an OpenAI-compatible chat endpoint. Default: Mistral
	// Small through fal.ai (OpenRouter). Mistral direct: LLM_BASE_URL=
	// https://api.mistral.ai/v1, LLM_AUTH=Bearer, LLM_MODEL=mistral-small-latest.
	LLMBaseURL string
	LLMAPIKey  string // server side only
	LLMAuth    string // Key (fal.ai) or Bearer
	LLMModel   string
}

const (
	DefaultLLMBaseURL = "https://fal.run/openrouter/router/openai/v1"
	DefaultLLMModel   = "mistralai/mistral-small-3.2-24b-instruct"
)

// Load reads the config from the environment and applies defaults.
func Load() (Config, error) {
	c := Config{
		Env:               getenv("LAAMO_ENV", "dev"),
		APIAddr:           getenv("API_ADDR", ":8787"),
		EurobaseURL:       getenv("EUROBASE_URL", DevEurobaseURL),
		EurobasePublicKey: os.Getenv("EUROBASE_PUBLIC_KEY"),
		EurobaseSecretKey: os.Getenv("EUROBASE_PRIVATE_KEY"),
		EurobaseProjectID: os.Getenv("EUROBASE_PROJECT_ID"),
		// BRAVESEARCH_PUBLIC_KEY is accepted for older local .env files.
		BraveSearchAPIKey: firstNonEmpty(os.Getenv("BRAVE_SEARCH_API_KEY"), os.Getenv("BRAVESEARCH_PUBLIC_KEY")),
		MasterKeys:        os.Getenv("LAAMO_MASTER_KEYS"),
		MasterKeyActive:   os.Getenv("LAAMO_MASTER_KEY_ACTIVE"),
		LLMBaseURL:        getenv("LLM_BASE_URL", DefaultLLMBaseURL),
		LLMAPIKey:         firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("FAL_KEY"), os.Getenv("FAL_AI_API_KEY")),
		LLMAuth:           getenv("LLM_AUTH", "Key"),
		LLMModel:          getenv("LLM_MODEL", DefaultLLMModel),
	}
	if c.EurobaseProjectID == "" && c.Env == "dev" && c.EurobaseURL == DevEurobaseURL {
		c.EurobaseProjectID = DevProjectID
	}
	if c.Env != "dev" {
		if c.EurobaseSecretKey == "" {
			return c, fmt.Errorf("EUROBASE_PRIVATE_KEY is required in %s", c.Env)
		}
		if c.EurobaseProjectID == "" {
			return c, fmt.Errorf("EUROBASE_PROJECT_ID is required in %s", c.Env)
		}
		if c.MasterKeys == "" {
			return c, fmt.Errorf("LAAMO_MASTER_KEYS is required in %s", c.Env)
		}
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
