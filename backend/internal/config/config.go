// Package config reads service settings from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds settings shared by all Laamo services.
type Config struct {
	Env               string // dev, staging, prod
	APIAddr           string
	EurobaseURL       string
	EurobasePublicKey string
	EurobaseSecretKey string // server side only
	BraveSearchAPIKey string
}

// Load reads the config from the environment and applies defaults.
func Load() (Config, error) {
	c := Config{
		Env:               getenv("LAAMO_ENV", "dev"),
		APIAddr:           getenv("API_ADDR", ":8787"),
		EurobaseURL:       getenv("EUROBASE_URL", "https://laamo.eurobase.app"),
		EurobasePublicKey: os.Getenv("EUROBASE_PUBLIC_KEY"),
		EurobaseSecretKey: os.Getenv("EUROBASE_PRIVATE_KEY"),
		// BRAVESEARCH_PUBLIC_KEY is accepted for older local .env files.
		BraveSearchAPIKey: firstNonEmpty(os.Getenv("BRAVE_SEARCH_API_KEY"), os.Getenv("BRAVESEARCH_PUBLIC_KEY")),
	}
	if c.Env != "dev" && c.EurobaseSecretKey == "" {
		return c, fmt.Errorf("EUROBASE_PRIVATE_KEY is required in %s", c.Env)
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
