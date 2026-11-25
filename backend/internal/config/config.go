// Package config reads service settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
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
	BraveRPS          float64 // Brave calls per second (free plan: 1)
	SearchCostEUR     float64 // per search, for the usage ledger
	SearchCacheDir    string  // dev only: cache search results here ("" = off)
	// Find businesses: the public OpenStreetMap servers by default; our own
	// for production volumes (#16).
	OSMNominatimURL string
	OSMOverpassURL  string
	GeoapifyAPIKey  string // when set, Find businesses uses Geoapify instead of Overpass
	// Incoming email (Cloudflare Email Worker → POST /api/inbound/email).
	InboundDomain string // helpers' addresses: <name>.<random>@InboundDomain
	InboundSecret string // shared with the Worker; signs each delivery (off when empty)
	// Outgoing email (Resend): only to the person themselves, e.g. the
	// waiting-list confirmation. Without a key, emails go to the log (dev).
	ResendAPIKey string
	MailFrom     string
	// Notifications (Web Push): the VAPID key pair that signs our pushes
	// (cmd/vapid makes one) and who we are to the push services. Without
	// the keys notifications are off. One pair for all environments that
	// share a database: a browser's subscription is tied to the key.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
	// Posting to social media (Posta): the partner key creates and manages
	// our users' Posta accounts. Server side only. There is no sandbox:
	// every environment talks to the real Posta. Without it social posting
	// is off.
	PostaPartnerKey string
	PostaURL        string // "" = posta.DefaultURL
	// SiteURL: the public site; links in emails point here.
	SiteURL    string
	FalKey     string // Draw a picture (fal.ai); the LLM key when that is fal's
	ImageModel string
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
	// The public demo (#75): signs proof-of-work challenges. Empty = a
	// random key per process (fine for one api instance).
	DemoSecret string
	// TrustProxy: take the client IP from X-Forwarded-For (set behind the
	// load balancer, never when the api is reachable directly).
	TrustProxy bool
	// InviteOnly: only people invited from the waiting list may use the app
	// (LAAMO_INVITE_ONLY=1/0; on by default outside dev).
	InviteOnly bool
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
		BraveRPS:          getfloat("BRAVE_RPS", 1),
		SearchCostEUR:     getfloat("SEARCH_COST_EUR", 0.005),
		OSMNominatimURL:   getenv("OSM_NOMINATIM_URL", ""), // "" = osm.DefaultNominatim
		OSMOverpassURL:    getenv("OSM_OVERPASS_URL", ""),  // "" = osm.DefaultOverpass
		GeoapifyAPIKey:    os.Getenv("GEOAPIFY_API_KEY"),
		InboundDomain:     getenv("INBOUND_DOMAIN", "in.laamo.app"),
		InboundSecret:     os.Getenv("INBOUND_SECRET"),
		ResendAPIKey:      os.Getenv("RESEND_API_KEY"),
		MailFrom:          getenv("MAIL_FROM", "Laamo <hello@notify.laamo.app>"),
		SiteURL:           getenv("SITE_URL", "http://localhost:5173"),
		PostaPartnerKey:   os.Getenv("POSTA_PARTNER_KEY"),
		PostaURL:          os.Getenv("POSTA_API_URL"),
		VAPIDPublicKey:    os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:   os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:      getenv("VAPID_SUBJECT", "https://laamo.app"),
		FalKey:            firstNonEmpty(os.Getenv("FAL_KEY"), os.Getenv("FAL_AI_API_KEY")),
		ImageModel:        getenv("IMAGE_MODEL", ""),
		MasterKeys:        os.Getenv("LAAMO_MASTER_KEYS"),
		MasterKeyActive:   os.Getenv("LAAMO_MASTER_KEY_ACTIVE"),
		LLMBaseURL:        getenv("LLM_BASE_URL", DefaultLLMBaseURL),
		LLMAPIKey:         firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("FAL_KEY"), os.Getenv("FAL_AI_API_KEY")),
		LLMAuth:           getenv("LLM_AUTH", "Key"),
		LLMModel:          getenv("LLM_MODEL", DefaultLLMModel),
		DemoSecret:        os.Getenv("LAAMO_DEMO_SECRET"),
		TrustProxy:        os.Getenv("LAAMO_TRUST_PROXY") == "1",
	}
	switch os.Getenv("LAAMO_INVITE_ONLY") {
	case "1":
		c.InviteOnly = true
	case "0":
		c.InviteOnly = false
	default:
		c.InviteOnly = c.Env != "dev"
	}
	if c.FalKey == "" && c.LLMAuth == "Key" {
		c.FalKey = c.LLMAPIKey // the brain goes through fal.ai: same key
	}
	if c.Env == "dev" {
		c.SearchCacheDir = getenv("SEARCH_CACHE_DIR", ".cache/search")
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

func getfloat(key string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return f
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
