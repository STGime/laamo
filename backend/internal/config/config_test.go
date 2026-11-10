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
