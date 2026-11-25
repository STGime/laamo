// Package push sends notifications to the browsers that turned them on
// (Web Push, RFC 8030): "there is something new in your Feed" on a phone or
// desktop, without a native app.
//
// A push goes to the browser maker's push service (Google, Apple, Mozilla,
// Microsoft), which hands it to the device. The content is encrypted for
// the browser (RFC 8291), so the push service can't read it; it does see
// that Laamo sent this device something, and when.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/STGime/laamo/backend/internal/store"
)

// ErrGone: the browser dropped this subscription (notifications turned off,
// site data cleared, app removed). Delete it.
var ErrGone = errors.New("push: subscription is gone")

// Message is what the notification shows. URL is where a tap leads (a path
// in the app); pushes with the same Tag replace each other on the device.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// Sender sends pushes, signed with the server's VAPID key pair.
type Sender struct {
	PublicKey  string // base64url, also given to browsers when they subscribe
	PrivateKey string
	// Subject tells push services who is sending: an https URL or an email
	// address (Apple refuses anything else).
	Subject string
	HTTP    *http.Client
	// Allow overrides which hosts may be sent to (tests). Default: the
	// push services of the big browsers.
	Allow func(host string) bool
}

// services: the push services of Chrome and friends, Safari, Firefox and
// Edge. The endpoint comes from the browser, so from the user: sending
// anywhere else would let them make our servers call any address.
var services = []string{
	"fcm.googleapis.com",
	".push.apple.com",
	".push.services.mozilla.com",
	".notify.windows.com",
}

// KnownService reports whether an endpoint is an https address at one of
// the browsers' push services.
func KnownService(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range services {
		if host == s || strings.HasPrefix(s, ".") && strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}

func (s *Sender) allowed(endpoint string) bool {
	if s.Allow == nil {
		return KnownService(endpoint)
	}
	u, err := url.Parse(endpoint)
	return err == nil && s.Allow(u.Host)
}

// recordSize is how many bytes every push has on the wire.
const recordSize = 1536

// client never follows redirects: a push service has no reason to send
// us elsewhere.
var client = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Send delivers m to one browser. ErrGone means the subscription is dead.
func (s *Sender) Send(ctx context.Context, sub store.PushSubscription, m Message, topic string) error {
	if !s.allowed(sub.Endpoint) {
		return fmt.Errorf("push: %q is not a known push service", hostOf(sub.Endpoint))
	}
	m.Title, m.Body = cut(m.Title, 60), cut(m.Body, 160)
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	hc := webpush.HTTPClient(client)
	if s.HTTP != nil {
		hc = s.HTTP
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body,
		&webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth}},
		&webpush.Options{
			HTTPClient: hc,
			// Every push is padded to this size (so its length says nothing).
			// The library's default of 4096 is too much for Firefox on
			// phones, whose push service takes about 2000 bytes; our
			// longest message is under 1100.
			RecordSize:      recordSize,
			Subscriber:      s.Subject,
			VAPIDPublicKey:  s.PublicKey,
			VAPIDPrivateKey: s.PrivateKey,
			// A phone that is off gets it when it's back, for two days.
			TTL:     48 * 3600,
			Urgency: webpush.UrgencyNormal,
			Topic:   topic,
		})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	}
	why, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	return fmt.Errorf("push: %s answered %d: %s", hostOf(sub.Endpoint), resp.StatusCode, strings.TrimSpace(string(why)))
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return "?"
}

// cut shortens s to at most n characters, on a character boundary.
func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}

// Notifier sends a person's notifications to all their browsers.
type Notifier struct {
	Store  store.Store
	Sender *Sender
	Log    *slog.Logger
}

// Notify sends m to every browser of the user and returns how many took
// it. Dead subscriptions are deleted on the way.
func (n *Notifier) Notify(ctx context.Context, userID string, m Message, topic string) (int, error) {
	subs, err := n.Store.PushSubscriptions(ctx, userID)
	if err != nil {
		return 0, err
	}
	sent := 0
	var last error
	for _, sub := range subs {
		switch err := n.Sender.Send(ctx, sub, m, topic); {
		case err == nil:
			sent++
		case errors.Is(err, ErrGone):
			if err := n.Store.DeletePushSubscription(ctx, sub.ID); err != nil {
				n.log().Warn("push: delete dead subscription", "id", sub.ID, "err", err)
			}
		default:
			last = err
			n.log().Warn("push: send", "user", userID, "err", err)
		}
	}
	return sent, last
}

// CardCreated tells the workspace's owner about a new Feed card, unless
// they are looking at the app anyway:
//   - runs they started by hand (Run now, a photo, a test) show their
//     result on the screen they are on;
//   - a result that comes after they answered an Ask me card in the Feed.
//
// It never fails: a push that doesn't go out is logged, the card is there.
func (n *Notifier) CardCreated(ctx context.Context, c store.FeedCard) {
	if n == nil || n.Sender == nil || c.UserID == "" {
		return
	}
	// The card is saved; a cancelled request or job shouldn't lose the push.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if c.RunID != nil {
		r, err := n.Store.GetRun(ctx, *c.RunID)
		if err == nil && (r.TestMode || r.TriggerType == store.TriggerManual || r.TriggerType == "test") {
			return
		}
		if err == nil && c.Type == "result" && n.asked(ctx, r.ID) {
			return
		}
	}
	m := Message{Title: "Laamo", Body: c.Title, URL: "/feed", Tag: c.ID}
	topic := ""
	if c.HelperID != nil {
		// One notification per helper: a newer one replaces the older.
		m.Tag = *c.HelperID
		topic = strings.ReplaceAll(*c.HelperID, "-", "") // 32 characters, the most a topic may have
		if h, err := n.Store.GetHelper(ctx, *c.HelperID); err == nil && h.Name != "" {
			m.Title = h.Name
		}
	}
	// Ask me cards carry the helper's name as their title and the question
	// as their text.
	if c.Title == m.Title && c.Body != nil {
		m.Body = *c.Body
	}
	if _, err := n.Notify(ctx, c.UserID, m, topic); err != nil {
		n.log().Warn("push: feed card", "card", c.ID, "err", err)
	}
}

// asked: the run stopped at an Ask me brick on its way.
func (n *Notifier) asked(ctx context.Context, runID string) bool {
	steps, err := n.Store.ListSteps(ctx, runID)
	if err != nil {
		return false
	}
	for _, s := range steps {
		if strings.HasPrefix(s.BrickType, "ask.") {
			return true
		}
	}
	return false
}

func (n *Notifier) log() *slog.Logger {
	if n.Log != nil {
		return n.Log
	}
	return slog.Default()
}

// notifying is a store whose new Feed cards also go out as notifications.
type notifying struct {
	store.Store
	n *Notifier
}

// Wrap returns st with every new Feed card announced through n. With a nil
// n (no VAPID keys) it returns st as it is.
func Wrap(st store.Store, n *Notifier) store.Store {
	if n == nil {
		return st
	}
	return notifying{Store: st, n: n}
}

func (s notifying) CreateFeedCard(ctx context.Context, c *store.FeedCard) error {
	if err := s.Store.CreateFeedCard(ctx, c); err != nil {
		return err // also ErrConflict: a retry of a card that was announced
	}
	s.n.CardCreated(ctx, *c)
	return nil
}

// New makes the notifier from the VAPID keys, or nil when they are missing
// (notifications are off then).
func New(st store.Store, publicKey, privateKey, subject string, log *slog.Logger) *Notifier {
	if publicKey == "" || privateKey == "" {
		return nil
	}
	return &Notifier{Store: st, Sender: &Sender{PublicKey: publicKey, PrivateKey: privateKey, Subject: subject}, Log: log}
}
