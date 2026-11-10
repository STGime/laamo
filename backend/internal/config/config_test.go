package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LAAMO_ENV", "")
	t.Setenv("API_ADDR", "")
	t.Setenv("EUROBASE_PRIVATE_KEY", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "dev" || c.APIAddr != ":8787" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoadRequiresSecretOutsideDev(t *testing.T) {
	t.Setenv("LAAMO_ENV", "prod")
	t.Setenv("EUROBASE_PROJECT_ID", "p1")
	t.Setenv("LAAMO_MASTER_KEYS", "v1:x")
	t.Setenv("EUROBASE_PRIVATE_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error without secret key in prod")
	}
}

func TestBraveKeyFallback(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	t.Setenv("BRAVESEARCH_PUBLIC_KEY", "old")
	c, _ := Load()
	if c.BraveSearchAPIKey != "old" {
		t.Fatalf("got %q", c.BraveSearchAPIKey)
	}
}

func TestProjectID(t *testing.T) {
	t.Setenv("LAAMO_ENV", "")
	t.Setenv("EUROBASE_URL", "")
	t.Setenv("EUROBASE_PROJECT_ID", "")
	c, _ := Load()
	if c.EurobaseProjectID != DevProjectID {
		t.Fatalf("dev default: got %q", c.EurobaseProjectID)
	}

	t.Setenv("EUROBASE_URL", "https://other.eurobase.app")
	if c, _ := Load(); c.EurobaseProjectID != "" {
		t.Fatalf("no default for another project: got %q", c.EurobaseProjectID)
	}

	t.Setenv("LAAMO_ENV", "prod")
	t.Setenv("EUROBASE_PRIVATE_KEY", "eb_sk_x")
	if _, err := Load(); err == nil {
		t.Fatal("prod needs EUROBASE_PROJECT_ID")
	}
	t.Setenv("EUROBASE_PROJECT_ID", "p1")
	if _, err := Load(); err == nil {
		t.Fatal("prod needs LAAMO_MASTER_KEYS")
	}
	t.Setenv("LAAMO_MASTER_KEYS", "v1:x")
	if c, err := Load(); err != nil || c.EurobaseProjectID != "p1" {
		t.Fatalf("explicit project id: %v %q", err, c.EurobaseProjectID)
	}
}

func TestLLMConfig(t *testing.T) {
	for _, k := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_AUTH", "LLM_MODEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("FAL_KEY", "fal-key")
	c, _ := Load()
	if c.LLMBaseURL != DefaultLLMBaseURL || c.LLMAPIKey != "fal-key" || c.LLMAuth != "Key" || c.LLMModel != DefaultLLMModel {
		t.Fatalf("fal.ai defaults: %+v", c)
	}
	t.Setenv("LLM_API_KEY", "mistral-key")
	t.Setenv("LLM_AUTH", "Bearer")
	if c, _ := Load(); c.LLMAPIKey != "mistral-key" || c.LLMAuth != "Bearer" {
		t.Fatalf("explicit key wins: %+v", c)
	}
}
