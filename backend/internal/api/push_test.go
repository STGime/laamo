package api_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/push"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// browserKeys makes the keys a browser hands over with its subscription.
func browserKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(secret)
}

func subscription(endpoint, p256dh, auth string) api.PushSubscription {
	s := api.PushSubscription{Endpoint: endpoint}
	s.Keys.P256dh, s.Keys.Auth = p256dh, auth
	return s
}

func TestPushOffWithoutKeys(t *testing.T) {
	e := newEnv(t, config.Config{})
	ctx := context.Background()
	c := e.client(t, "anna")
	if r := must(c.GetPushKeyWithResponse(ctx)); r.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("key: %d %s", r.StatusCode(), r.Body)
	}
	p, a := browserKeys(t)
	if r := must(c.SavePushSubscriptionWithResponse(ctx, subscription("https://fcm.googleapis.com/fcm/send/x", p, a))); r.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("save: %d %s", r.StatusCode(), r.Body)
	}
	if r := must(e.client(t, "").GetPushKeyWithResponse(ctx)); r.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", r.StatusCode())
	}
}

func TestPushSubscriptions(t *testing.T) {
	st := memory.New()
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	n := &push.Notifier{Store: st, Sender: &push.Sender{PublicKey: public, PrivateKey: private, Subject: "https://laamo.app"}}
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna, "ben": ben}, Push: n}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	asAnna, asBen := e.client(t, "anna"), e.client(t, "ben")
	p, a := browserKeys(t)

	if r := must(asAnna.GetPushKeyWithResponse(ctx)); r.StatusCode() != 200 || r.JSON200.PublicKey != public {
		t.Fatalf("key: %d %s", r.StatusCode(), r.Body)
	}

	// Only a browser's push service may be an endpoint, and only real keys.
	short := base64.RawURLEncoding.EncodeToString(make([]byte, 65)) // 65 bytes, but not a point
	for name, s := range map[string]api.PushSubscription{
		"our own server":   subscription(srv.URL+"/api/healthz", p, a),
		"any https site":   subscription("https://example.com/push", p, a),
		"a metadata host":  subscription("https://169.254.169.254/x", p, a),
		"plain http":       subscription("http://fcm.googleapis.com/fcm/send/x", p, a),
		"not a public key": subscription("https://fcm.googleapis.com/fcm/send/x", short, a),
		"not a secret":     subscription("https://fcm.googleapis.com/fcm/send/x", p, base64.RawURLEncoding.EncodeToString(make([]byte, 20))),
	} {
		if r := must(asAnna.SavePushSubscriptionWithResponse(ctx, s)); r.StatusCode() != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, r.StatusCode(), r.Body)
		}
	}
	if subs, _ := st.PushSubscriptions(ctx, anna.ID); len(subs) != 0 {
		t.Fatalf("saved %d subscriptions", len(subs))
	}

	phone := "https://fcm.googleapis.com/fcm/send/annas-phone"
	for range 2 { // turning it on twice keeps one row
		if r := must(asAnna.SavePushSubscriptionWithResponse(ctx, subscription(phone, p, a))); r.StatusCode() != http.StatusNoContent {
			t.Fatalf("save: %d %s", r.StatusCode(), r.Body)
		}
	}
	subs, _ := st.PushSubscriptions(ctx, anna.ID)
	if len(subs) != 1 || subs[0].Endpoint != phone || subs[0].P256dh != p {
		t.Fatalf("subscriptions: %+v", subs)
	}

	// Ben can't turn Anna's phone off, nor test it.
	if r := must(asBen.DeletePushSubscriptionWithResponse(ctx, &api.DeletePushSubscriptionParams{Endpoint: phone})); r.StatusCode() != http.StatusNoContent {
		t.Fatalf("ben deletes: %d", r.StatusCode())
	}
	if subs, _ := st.PushSubscriptions(ctx, anna.ID); len(subs) != 1 {
		t.Fatal("Ben removed Anna's subscription")
	}
	if r := must(asBen.TestPushWithResponse(ctx, api.TestPushJSONRequestBody{Endpoint: phone})); r.StatusCode() != http.StatusNotFound {
		t.Fatalf("ben tests: %d %s", r.StatusCode(), r.Body)
	}

	// The same phone, now Ben's (he signed in on it): it moves to him.
	if r := must(asBen.SavePushSubscriptionWithResponse(ctx, subscription(phone, p, a))); r.StatusCode() != http.StatusNoContent {
		t.Fatalf("ben saves: %d", r.StatusCode())
	}
	as, _ := st.PushSubscriptions(ctx, anna.ID)
	bs, _ := st.PushSubscriptions(ctx, ben.ID)
	if len(as) != 0 || len(bs) != 1 {
		t.Fatalf("after Ben signed in on the phone: anna %d, ben %d", len(as), len(bs))
	}
	if r := must(asBen.DeletePushSubscriptionWithResponse(ctx, &api.DeletePushSubscriptionParams{Endpoint: phone})); r.StatusCode() != http.StatusNoContent {
		t.Fatalf("delete: %d", r.StatusCode())
	}
	if bs, _ := st.PushSubscriptions(ctx, ben.ID); len(bs) != 0 {
		t.Fatalf("still on: %+v", bs)
	}

	// More browsers than allowed: the oldest gives way.
	for i := range api.MaxPushSubscriptions + 2 {
		s := subscription(fmt.Sprintf("https://fcm.googleapis.com/fcm/send/device-%02d", i), p, a)
		if r := must(asAnna.SavePushSubscriptionWithResponse(ctx, s)); r.StatusCode() != http.StatusNoContent {
			t.Fatalf("device %d: %d %s", i, r.StatusCode(), r.Body)
		}
	}
	subs, _ = st.PushSubscriptions(ctx, anna.ID)
	if len(subs) != api.MaxPushSubscriptions || subs[0].Endpoint != "https://fcm.googleapis.com/fcm/send/device-02" {
		t.Fatalf("%d subscriptions, oldest %s", len(subs), subs[0].Endpoint)
	}
}

func TestPushTest(t *testing.T) {
	var hits atomic.Int32
	status := http.StatusCreated
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(service.Close)
	st := memory.New()
	private, public, _ := webpush.GenerateVAPIDKeys()
	n := &push.Notifier{Store: st, Sender: &push.Sender{PublicKey: public, PrivateKey: private, Subject: "https://laamo.app",
		HTTP: service.Client(), Allow: func(string) bool { return true }}}
	srv := httptest.NewServer(api.NewServer(api.Deps{Store: st, Auth: fakeAuth{"anna": anna}, Push: n}).Handler())
	t.Cleanup(srv.Close)
	c := env{url: srv.URL, store: st}.client(t, "anna")
	ctx := context.Background()
	p, a := browserKeys(t)
	endpoint := service.URL + "/send/1"
	if r := must(c.SavePushSubscriptionWithResponse(ctx, subscription(endpoint, p, a))); r.StatusCode() != http.StatusNoContent {
		t.Fatalf("save: %d %s", r.StatusCode(), r.Body)
	}
	de := api.De
	if r := must(c.TestPushWithResponse(ctx, api.TestPushJSONRequestBody{Endpoint: endpoint, Locale: &de})); r.StatusCode() != http.StatusNoContent || hits.Load() != 1 {
		t.Fatalf("test: %d %s (%d sent)", r.StatusCode(), r.Body, hits.Load())
	}
	// The browser dropped the subscription meanwhile: we forget it too.
	status = http.StatusGone
	if r := must(c.TestPushWithResponse(ctx, api.TestPushJSONRequestBody{Endpoint: endpoint})); r.StatusCode() != http.StatusNotFound {
		t.Fatalf("gone: %d %s", r.StatusCode(), r.Body)
	}
	if subs, _ := st.PushSubscriptions(ctx, anna.ID); len(subs) != 0 {
		t.Fatalf("kept a dead subscription: %+v", subs)
	}
}
