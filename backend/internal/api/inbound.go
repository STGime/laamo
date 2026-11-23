package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	guuid "github.com/google/uuid"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/images"
	"github.com/STGime/laamo/backend/internal/mail"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// Incoming email. Cloudflare Email Routing hands every mail for
// *@in.laamo.app to our Email Worker (infra/cloudflare/inbound-mail), which
// POSTs the raw email here, signed with INBOUND_SECRET:
//
//	X-Laamo-Timestamp: unix seconds
//	X-Laamo-Signature: hex HMAC-SHA256 of "<timestamp>.<body>"
//	X-Laamo-To:        the address it was sent to
//
// Answers the Worker acts on: 202 a run started (or the mail was already
// here, or was ignored on purpose: no bounce to strangers), 404 no such
// address (the Worker rejects the mail), 410 the helper is off, 413 too big,
// 422 not an email, 401 bad signature.
const inboundPath = BaseURL + "/inbound/email"

// MaxMailsPerDay: emails one helper takes in 24 hours; more are ignored
// (a mail loop or spam shouldn't eat the brain allowance).
const MaxMailsPerDay = 30

// inboundSkew: how old a delivery may be (replays of a captured request).
const inboundSkew = 5 * time.Minute

// mailRunSpace makes run ids from emails: the same email twice is one run.
var mailRunSpace = guuid.MustParse("6f1c2f0e-6a43-4c55-9d0e-6c61616d6f31")

func (s *Server) serveInbound(w http.ResponseWriter, r *http.Request) {
	answer := func(status int, outcome string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"outcome": outcome})
	}
	if s.cfg.InboundSecret == "" {
		answer(http.StatusNotFound, "off")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, mail.MaxInbound+1))
	if err != nil {
		answer(http.StatusBadRequest, "unreadable")
		return
	}
	if !s.inboundSigned(r.Header, raw, time.Now()) {
		answer(http.StatusUnauthorized, "bad_signature")
		return
	}
	if len(raw) > mail.MaxInbound {
		answer(http.StatusRequestEntityTooLarge, "too_big")
		return
	}
	to, tag, err := mail.Recipient(r.Header.Get("X-Laamo-To"))
	if err != nil || !strings.HasSuffix(to, "@"+s.cfg.InboundDomain) {
		answer(http.StatusNotFound, "no_such_address")
		return
	}
	status, outcome, err := s.takeMail(r.Context(), to, tag, raw)
	if err != nil {
		s.log.Error("inbound mail", "to", to, "err", err)
		// The Worker lets Cloudflare try again later.
		answer(http.StatusServiceUnavailable, "try_later")
		return
	}
	s.log.Info("inbound mail", "to", to, "outcome", outcome, "bytes", len(raw))
	answer(status, outcome)
}

// inboundSigned checks the Worker's signature and that it's recent.
func (s *Server) inboundSigned(h http.Header, body []byte, now time.Time) bool {
	ts, err := strconv.ParseInt(h.Get("X-Laamo-Timestamp"), 10, 64)
	if err != nil {
		return false
	}
	if d := now.Sub(time.Unix(ts, 0)); d > inboundSkew || d < -inboundSkew {
		return false
	}
	got, err := hex.DecodeString(h.Get("X-Laamo-Signature"))
	if err != nil {
		return false
	}
	return hmac.Equal(got, InboundSignature(s.cfg.InboundSecret, ts, body))
}

// InboundSignature is what the Worker sends (and cmd/sendmail, for testing).
func InboundSignature(secret string, ts int64, body []byte) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(m, "%d.", ts)
	_, _ = m.Write(body)
	return m.Sum(nil)
}

// takeMail hands an email to the helper it was sent to. tag is the address's
// "+tag": the helper's forwarding key, or anything else (then ignored).
func (s *Server) takeMail(ctx context.Context, to, tag string, raw []byte) (int, string, error) {
	t, err := s.store.TriggerByAddress(ctx, to)
	if errors.Is(err, store.ErrNotFound) || (err == nil && t.Type != store.TriggerEmail) {
		return http.StatusNotFound, "no_such_address", nil
	}
	if err != nil {
		return 0, "", err
	}
	h, err := s.store.GetHelper(ctx, t.HelperID)
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound, "no_such_address", nil
	}
	if err != nil {
		return 0, "", err
	}
	if h.Status != "active" || h.CurrentVersionID == nil {
		return http.StatusGone, "helper_off", nil
	}
	sum := sha256.Sum256(raw)
	runID := guuid.NewSHA1(mailRunSpace, append([]byte(h.ID+"/"), sum[:]...)).String()
	if _, err := s.store.GetRun(ctx, runID); err == nil {
		return http.StatusAccepted, "duplicate", nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return 0, "", err
	}

	m, err := mail.Parse(raw)
	if err != nil {
		return http.StatusUnprocessableEntity, "not_an_email", nil
	}
	senders, err := s.mailSenders(ctx, *h.CurrentVersionID)
	if err != nil {
		return 0, "", err
	}
	// Did the From address's domain sign the mail? Anyone can write any
	// From, so only then is the sender who they say they are. The run gets
	// the answer too: the user sees whether the sender was checked.
	signed := mail.SignedBy(raw, m.From, nil)
	// Setting up Gmail forwarding: Gmail mails a code to type in. That's
	// for the user, not a run.
	if m.From == mail.GmailForwarding && signed {
		if code, requester, ok := mail.GmailForwardCode(m, s.cfg.InboundDomain); ok {
			s.noteForwardCode(ctx, h, code, requester, time.Now())
			return http.StatusAccepted, "forward_confirmation", nil
		}
	}
	// Sent to the forwarding address: the user's own forwarding rule (only
	// they know the key), so any original sender is fine. Otherwise Only
	// from these senders: the address must be on the list and signed.
	forwarded := mail.IsForwardKey(t, tag)
	if !forwarded && len(senders) > 0 && (!mail.Allowed(m.From, senders) || !signed) {
		s.log.Info("inbound mail ignored: sender not on the helper's list or not signed", "helper", h.ID, "from", m.From)
		s.noteIgnored(ctx, h, m, senders, signed, time.Now())
		return http.StatusAccepted, "sender_not_allowed", nil
	}
	if n, err := s.mailsToday(ctx, h.ID); err != nil {
		return 0, "", err
	} else if n >= MaxMailsPerDay {
		s.log.Warn("inbound mail ignored: daily limit", "helper", h.ID, "limit", MaxMailsPerDay)
		return http.StatusAccepted, "daily_limit", nil
	}
	if s.blobs == nil || s.queue == nil {
		return 0, "", errors.New("storage or job queue not configured")
	}
	path := fmt.Sprintf("mail/%s/%s/%s.eml", h.WorkspaceID, h.ID, hex.EncodeToString(sum[:12]))
	if err := s.blobs.Put(ctx, path, raw); err != nil {
		return 0, "", err
	}

	data := map[string]any{"from": m.From, "from_name": m.FromName, "from_signed": yesNo(signed), "subject": m.Subject, "text": m.Text, "mail": path}
	if !m.Date.IsZero() {
		data["date"] = m.Date.UTC().Format(time.RFC3339)
	}
	if len(m.Attachments) > 0 {
		names := make([]string, 0, len(m.Attachments))
		for _, a := range m.Attachments {
			if a.Name != "" {
				names = append(names, a.Name)
			}
		}
		data["attachments"] = strings.Join(names, ", ")
	}
	// The email's pictures, for Read the photo: stored like the pictures a
	// helper draws, so cards and lists can show them.
	if photos := s.keepPhotos(ctx, h, runID, m.Pictures()); len(photos) > 0 {
		data["photos"] = strings.Join(photos, ",")
	}
	eng := &engine.Engine{Store: s.store, Catalog: catalog.Default(), Log: s.log}
	run, err := eng.StartRun(ctx, engine.StartRequest{RunID: runID, HelperID: h.ID, TriggerType: store.TriggerEmail, TriggerData: data})
	if errors.Is(err, engine.ErrNotRunnable) {
		return http.StatusGone, "helper_off", nil
	}
	if err != nil {
		return 0, "", err
	}
	if _, err := s.queue.Enqueue(ctx, queue.NewJob{WorkspaceID: run.WorkspaceID, Kind: engine.JobKind,
		Payload: map[string]string{"run_id": run.ID}}); err != nil {
		return 0, "", err
	}
	return http.StatusAccepted, "run_started", nil
}

// keepPhotos stores an email's pictures for the run (upright, shrunk, as
// JPEG) and returns their paths. A picture that can't be read or stored is
// left out: the email still starts its run.
func (s *Server) keepPhotos(ctx context.Context, h store.Helper, runID string, pics []mail.Attachment) []string {
	var paths []string
	for i, a := range pics {
		jpg, err := images.Photo(a.Data)
		if err != nil {
			s.log.Info("inbound mail: a picture was left out", "helper", h.ID, "name", a.Name, "err", err)
			continue
		}
		path := photoPath(h.WorkspaceID, runID, i+1)
		if err := s.blobs.Put(ctx, path, jpg); err != nil {
			s.log.Warn("inbound mail: couldn't store a picture", "helper", h.ID, "err", err)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// photoPath is where a run's n-th photo is stored. GetImage serves
// everything under images/<workspace>/ to that workspace's members.
func photoPath(workspaceID, dir string, n int) string {
	return fmt.Sprintf("images/%s/%s/p%d.jpg", workspaceID, dir, n)
}

// mailSenders is the Only from these senders list of a helper version.
func (s *Server) mailSenders(ctx context.Context, versionID string) ([]string, error) {
	v, err := s.store.GetHelperVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	var st catalog.Stack
	if err := json.Unmarshal(v.Stack, &st); err != nil || len(st.Bricks) == 0 || st.Bricks[0].Type != "when.email" {
		return nil, err
	}
	var out []string
	switch list := st.Bricks[0].Config["senders"].(type) {
	case []any:
		for _, x := range list {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
	case string: // tags typed as one line
		out = strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
	}
	return out, nil
}

// mailsToday counts the helper's email runs in the last 24 hours.
func (s *Server) mailsToday(ctx context.Context, helperID string) (int, error) {
	runs, err := s.store.ListRuns(ctx, store.RunFilter{HelperID: helperID, Limit: MaxMailsPerDay * 3})
	if err != nil {
		return 0, err
	}
	since, n := time.Now().Add(-24*time.Hour), 0
	for _, r := range runs {
		if r.TriggerType == store.TriggerEmail && r.CreatedAt.After(since) {
			n++
		}
	}
	return n, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
