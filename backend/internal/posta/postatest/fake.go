// Package postatest is a fake Posta for tests: the partner API and the
// parts of the normal API Laamo uses, in memory.
package postatest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/posta"
)

const (
	// PartnerKey is the key the fake accepts.
	PartnerKey = "postapk_test"
	// Origin is the partner's registered site: connects must come back to it.
	Origin = "https://laamo.test"
	// BlueskyPassword is the only app password the fake's Bluesky takes.
	BlueskyPassword = "abcd-efgh-ijkl-mnop"
)

type account struct {
	email, externalID, token string
	plan                     posta.Plan
	channels                 []posta.Channel
}

// Fake is the server. Set Down to make every call fail with a 500.
type Fake struct {
	URL  string
	Down bool

	mu       sync.Mutex
	n        int
	accounts map[string]*account // by external id
	own      map[string]*account // people's own accounts, by token
	taken    map[string]bool     // emails with an own account
	Calls    []string            // "METHOD path", in order
	media    map[string]*media
	posts    []*FakePost
	// FailCreate: the next n "create post" calls answer 500 (Posta is in
	// trouble), LoseCreate: the next one makes the post but the answer is
	// lost on the way (a 502), as a timeout would.
	FailCreate, LoseCreate int
}

type media struct {
	owner  *account
	mime   string
	data   []byte
	looked int // how often its state was read after the upload
}

// FakePost is a post made at the fake.
type FakePost struct {
	ID       string
	Caption  string
	Channels []string
	MediaIDs []string
	Status   string // draft, scheduled, processing
	At       *time.Time
	Publish  int // how often it was told to go out
	owner    *account
}

// Posts returns the posts made so far.
func (f *Fake) Posts() []FakePost {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakePost, len(f.posts))
	for i, p := range f.posts {
		out[i] = *p
	}
	return out
}

// Picture returns the bytes uploaded as this media id.
func (f *Fake) Picture(id string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m := f.media[id]; m != nil {
		return m.data
	}
	return nil
}

// New starts a fake Posta and returns it with a service that talks to it.
func New(t *testing.T) (*Fake, *posta.Service) {
	t.Helper()
	f := &Fake{accounts: map[string]*account{}, own: map[string]*account{}, taken: map[string]bool{}, media: map[string]*media{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f, posta.New(PartnerKey, srv.URL)
}

// OwnAccount adds a Posta account a person made themselves, and returns
// its API key.
func (f *Fake) OwnAccount(email string, channels ...posta.Channel) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	token := fmt.Sprintf("posta_own_%d", f.n)
	f.own[token] = &account{email: email, token: token, channels: channels}
	f.taken[email] = true
	return token
}

// Token returns the current token of the account made for a Laamo user
// ("" when there is none).
func (f *Fake) Token(externalID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a := f.accounts[externalID]; a != nil {
		return a.token
	}
	return ""
}

// SetChannels sets the channels of the account made for a Laamo user.
func (f *Fake) SetChannels(externalID string, channels ...posta.Channel) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a := f.accounts[externalID]; a != nil {
		a.channels = channels
	}
}

func fail(w http.ResponseWriter, status int, code, msg string, reason ...string) {
	body := map[string]any{"error": msg, "code": code}
	if len(reason) > 0 {
		body["details"] = map[string]any{"reason": reason}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func ok(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *account) out(created bool, trial string, withToken bool) posta.Account {
	o := posta.Account{Created: created, Trial: trial, UserID: "posta-" + a.externalID, ExternalUserID: a.externalID,
		Plan: a.plan, ConnectedAccounts: len(a.channels)}
	if withToken {
		o.APIToken = a.token
	}
	return o
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, r.Method+" "+r.URL.Path)
	if f.Down {
		fail(w, http.StatusInternalServerError, "INTERNAL", "down")
		return
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	path := r.URL.Path
	if rest, isPartner := strings.CutPrefix(path, "/partners"); isPartner {
		if bearer != PartnerKey {
			fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid partner key")
			return
		}
		f.partner(w, r, rest)
		return
	}
	if id, isUpload := strings.CutPrefix(path, "/upload/"); isUpload && r.Method == http.MethodPut {
		m := f.media[id]
		if m == nil || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != m.mime {
			fail(w, http.StatusForbidden, "FORBIDDEN", "bad upload")
			return
		}
		m.data, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet && path == "/social-accounts/platforms" {
		ok(w, http.StatusOK, map[string]any{"platforms": []map[string]any{
			{"id": "instagram", "name": "Instagram", "isConfigured": true, "supportsImage": true, "supportsVideo": true},
			{"id": "youtube", "name": "YouTube", "isConfigured": true, "supportsImage": false, "supportsVideo": true},
			{"id": "x", "name": "X", "isConfigured": false, "supportsImage": true, "supportsVideo": true},
			{"id": "bluesky", "name": "Bluesky", "isConfigured": true, "authType": "app_password", "supportsImage": true, "supportsVideo": true},
		}})
		return
	}
	// The normal API, as one user.
	var a *account
	if own := f.own[bearer]; own != nil {
		a = own
	}
	for _, x := range f.accounts {
		if x.token == bearer {
			a = x
		}
	}
	if a == nil {
		fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or revoked API token")
		return
	}
	_, isOwn := f.own[bearer]
	switch {
	case r.Method == http.MethodGet && path == "/social-accounts":
		// As the real Posta sends them: ids are numbers, a missing display
		// name is null.
		list := []map[string]any{}
		for _, c := range a.channels {
			m := map[string]any{"platform": c.Platform, "username": c.Username, "displayName": nil,
				"isActive": c.IsActive, "connectionError": nil}
			if n, err := strconv.Atoi(c.ID); err == nil {
				m["id"] = n
			} else {
				m["id"] = c.ID
			}
			if c.DisplayName != "" {
				m["displayName"] = c.DisplayName
			}
			if c.ConnectionError != "" {
				m["connectionError"] = c.ConnectionError
			}
			list = append(list, m)
		}
		ok(w, http.StatusOK, map[string]any{"accounts": list})
	case r.Method == http.MethodPost && path == "/social-accounts/oauth/initiate":
		var in struct {
			Platform    string `json:"platform"`
			RedirectURL string `json:"redirectUrl"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		// Only for accounts the partner made, and back to its own site.
		if isOwn || !strings.HasPrefix(in.RedirectURL, Origin+"/") {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "redirectUrl is not allowed")
			return
		}
		if f.refuse(w, a, true) {
			return
		}
		ok(w, http.StatusOK, map[string]any{"authorizationUrl": "https://" + in.Platform + ".example/oauth?state=s1", "state": "s1"})
	case r.Method == http.MethodPost && path == "/social-accounts/bluesky/connect":
		var in struct {
			Identifier  string `json:"identifier"`
			AppPassword string `json:"appPassword"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if f.refuse(w, a, true) {
			return
		}
		if in.AppPassword != BlueskyPassword {
			fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication failed")
			return
		}
		f.n++
		a.channels = append(a.channels, posta.Channel{ID: strconv.Itoa(1000 + f.n), Platform: "bluesky", Username: in.Identifier, IsActive: true})
		ok(w, http.StatusOK, map[string]any{"success": true})
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/social-accounts/"):
		id := strings.TrimPrefix(path, "/social-accounts/")
		n := len(a.channels)
		a.channels = slices.DeleteFunc(a.channels, func(c posta.Channel) bool { return c.ID == id })
		if len(a.channels) == n {
			fail(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "/media/create-upload-url":
		var in struct {
			Name string `json:"name"`
			Mime string `json:"mime_type"`
			Size int    `json:"size_bytes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Name == "" || in.Size <= 0 || !slices.Contains([]string{"image/jpeg", "image/png", "image/webp"}, in.Mime) {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid file type or size")
			return
		}
		f.n++
		id := fmt.Sprintf("media-%d", f.n)
		f.media[id] = &media{owner: a, mime: in.Mime}
		ok(w, http.StatusCreated, map[string]any{"media_id": id, "upload_url": f.URL + "/upload/" + id})
	case strings.HasPrefix(path, "/media/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, "/media/"), "/")
		m := f.media[id]
		if m == nil || m.owner != a {
			fail(w, http.StatusNotFound, "NOT_FOUND", "Media not found")
			return
		}
		switch {
		case r.Method == http.MethodPost && action == "confirm-upload" && m.data != nil:
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && action == "":
			// Processing takes a moment: ready from the second look on.
			m.looked++
			status := "processing"
			if m.looked > 1 {
				status = "completed"
			}
			ok(w, http.StatusOK, map[string]any{"id": id, "status": status})
		default:
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "not uploaded yet")
		}
	case r.Method == http.MethodPost && path == "/posts":
		f.createPost(w, r, a)
	case strings.HasPrefix(path, "/posts/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, "/posts/"), "/")
		i := slices.IndexFunc(f.posts, func(p *FakePost) bool { return p.ID == id && p.owner == a })
		if i < 0 {
			fail(w, http.StatusNotFound, "NOT_FOUND", "Post not found")
			return
		}
		post := f.posts[i]
		switch {
		case r.Method == http.MethodGet && action == "":
			ok(w, http.StatusOK, map[string]any{"id": post.ID, "status": post.Status})
		case r.Method == http.MethodPost && action == "publish":
			if f.refuse(w, a, false) {
				return
			}
			if post.Status != "draft" {
				fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Post cannot be published")
				return
			}
			post.Status, post.Publish = "processing", post.Publish+1
			ok(w, http.StatusOK, map[string]any{"id": post.ID, "status": post.Status})
		default:
			fail(w, http.StatusNotFound, "NOT_FOUND", "no such route in the fake")
		}
	default:
		fail(w, http.StatusNotFound, "NOT_FOUND", "no such route in the fake")
	}
}

func (f *Fake) createPost(w http.ResponseWriter, r *http.Request, a *account) {
	if f.FailCreate > 0 {
		f.FailCreate--
		fail(w, http.StatusInternalServerError, "INTERNAL", "try later")
		return
	}
	var in struct {
		Caption  string     `json:"caption"`
		MediaIDs []string   `json:"mediaIds"`
		Channels []any      `json:"socialAccountIds"`
		At       *time.Time `json:"scheduledAt"`
		IsDraft  *bool      `json:"isDraft"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Channels) == 0 {
		fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}
	if f.refuse(w, a, false) {
		return
	}
	post := &FakePost{Caption: in.Caption, MediaIDs: in.MediaIDs, At: in.At, Status: "draft", owner: a}
	for _, c := range in.Channels {
		// Whole numbers, or numeric text; and one of this account's own.
		var id string
		switch v := c.(type) {
		case float64:
			id = strconv.FormatFloat(v, 'f', -1, 64)
		case string:
			id = v
		}
		if _, err := strconv.Atoi(id); err != nil || !slices.ContainsFunc(a.channels, func(ch posta.Channel) bool { return ch.ID == id }) {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "socialAccountIds: not one of your accounts")
			return
		}
		post.Channels = append(post.Channels, id)
	}
	for _, id := range in.MediaIDs {
		if m := f.media[id]; m == nil || m.owner != a || m.looked < 2 {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Media is not ready")
			return
		}
	}
	if in.At != nil && (in.IsDraft == nil || !*in.IsDraft) {
		post.Status = "scheduled"
	}
	f.n++
	post.ID = fmt.Sprintf("post-%d", f.n)
	f.posts = append(f.posts, post)
	if f.LoseCreate > 0 {
		f.LoseCreate--
		w.WriteHeader(http.StatusBadGateway) // made, but the caller never learns its id
		return
	}
	ok(w, http.StatusCreated, map[string]any{"id": post.ID, "status": post.Status})
}

// refuse answers 403 when the account's plan doesn't allow the call
// (connecting also needs room under the channel limit).
func (f *Fake) refuse(w http.ResponseWriter, a *account, connecting bool) bool {
	reason := ""
	switch max := a.plan.Limits.MaxSocialAccounts; {
	case a.externalID == "": // a person's own account: Posta's business
	case !a.plan.IsActive && a.plan.Type == "trial":
		reason = "trial_expired"
	case !a.plan.IsActive:
		reason = "plan_inactive"
	case connecting && max != nil && len(a.channels) >= *max:
		reason = "account_limit"
	}
	if reason == "" {
		return false
	}
	fail(w, http.StatusForbidden, "PLAN_LIMIT_EXCEEDED", "Plan limit", reason)
	return true
}

// EndTrial makes the trial of a Laamo user's account run out.
func (f *Fake) EndTrial(externalID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a := f.accounts[externalID]; a != nil {
		a.plan.IsActive = false
		a.plan.DaysRemaining = new(0)
	}
}

func (f *Fake) partner(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodPost && rest == "/accounts":
		var in posta.NewAccount
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Email == "" || in.ExternalUserID == "" || in.TermsAcceptedAt.IsZero() {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
			return
		}
		if f.taken[in.Email] {
			fail(w, http.StatusConflict, "CONFLICT", "Account exists", "account_exists")
			return
		}
		a := f.accounts[in.ExternalUserID]
		created := a == nil
		if a != nil && a.email != in.Email {
			fail(w, http.StatusConflict, "CONFLICT", "Email mismatch", "email_mismatch")
			return
		}
		trial := "existing"
		if created {
			end := time.Now().Add(5 * 24 * time.Hour).UTC()
			days, ten := 5, 10
			a = &account{email: in.Email, externalID: in.ExternalUserID,
				plan: posta.Plan{Type: "trial", IsActive: true, TrialEndsAt: &end, ValidUntil: &end, DaysRemaining: &days}}
			a.plan.Limits.MaxSocialAccounts = &ten
			f.accounts[in.ExternalUserID] = a
			trial = "started"
		}
		f.n++
		a.token = fmt.Sprintf("posta_issued_%d", f.n) // a new token on every call
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		ok(w, status, a.out(created, trial, true))
		return
	case r.Method == http.MethodGet && rest == "/usage":
		ok(w, http.StatusOK, posta.Usage{AccountsTotal: len(f.accounts), AccountsLinked: len(f.accounts)})
		return
	}
	id, action, _ := strings.Cut(strings.TrimPrefix(rest, "/accounts/"), "/")
	a := f.accounts[id]
	if !strings.HasPrefix(rest, "/accounts/") || a == nil {
		fail(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		ok(w, http.StatusOK, a.out(false, "", false))
	case r.Method == http.MethodDelete && action == "":
		delete(f.accounts, id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut && action == "plan":
		var in struct {
			Plan       string     `json:"plan"`
			ValidUntil *time.Time `json:"valid_until"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Plan != "starter" && in.Plan != "professional" && in.Plan != "none" {
			fail(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
			return
		}
		a.plan = posta.Plan{Type: in.Plan, IsActive: in.Plan != "none", ValidUntil: in.ValidUntil}
		ok(w, http.StatusOK, a.out(false, "", false))
	case r.Method == http.MethodPost && action == "token":
		f.n++
		a.token = fmt.Sprintf("posta_issued_%d", f.n)
		ok(w, http.StatusOK, map[string]string{"api_token": a.token})
	default:
		fail(w, http.StatusNotFound, "NOT_FOUND", "no such route in the fake")
	}
}
