package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResend(t *testing.T) {
	var got map[string]any
	var auth, idem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, idem = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["to"].([]any)[0] == "bad@example.com" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"invalid to"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"e1"}`))
	}))
	defer srv.Close()
	r := &Resend{APIKey: "re_test", From: "Laamo <hello@notify.laamo.app>", URL: srv.URL}

	err := r.Send(context.Background(), Outgoing{To: "anna@example.com", Subject: "Hi", Text: "t", HTML: "<p>t</p>", IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_test" || idem != "k1" || got["from"] != "Laamo <hello@notify.laamo.app>" || got["subject"] != "Hi" {
		t.Fatalf("request: auth=%q idem=%q body=%v", auth, idem, got)
	}
	err = r.Send(context.Background(), Outgoing{To: "bad@example.com"})
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("error status: %v", err)
	}
}
