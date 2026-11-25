// Package posta talks to Posta (getposta.app), which posts to social
// networks for Laamo's users.
//
// Two credentials, both server side only:
//   - the partner key (postapk_…): creates Posta accounts for Laamo users,
//     sets their plan, rotates their token, deletes them (Partner);
//   - a user token (posta_…): the normal Posta API as one user (User).
//
// There is no sandbox: every call goes to the real Posta.
package posta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is Posta's API.
const DefaultURL = "https://api.getposta.app/v1"

var (
	// ErrAccountExists: the email has a Posta account Laamo didn't create
	// (or let go of). The person pastes their own Posta API key instead.
	ErrAccountExists = errors.New("posta: this email already has a Posta account")
	// ErrEmailMismatch: this Laamo user has a Posta account under another
	// email. A bug in our id mapping, never the user's fault.
	ErrEmailMismatch = errors.New("posta: this user has an account under another email")
	// ErrNotFound: no such account, channel or post.
	ErrNotFound = errors.New("posta: not found")
	// ErrUnauthorized: the key or token is wrong or was revoked.
	ErrUnauthorized = errors.New("posta: key or token not accepted")
	// ErrPlan: the account's plan doesn't allow this (see Error.Reason:
	// trial_expired, plan_inactive, account_limit).
	ErrPlan = errors.New("posta: the plan doesn't allow this")
)

// Error is an answer from Posta that isn't a success.
type Error struct {
	Status  int
	Code    string // e.g. NOT_FOUND, PLAN_LIMIT_EXCEEDED
	Message string
	Reason  string // details.reason[0], e.g. account_exists
	kind    error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("posta: %d %s", e.Status, e.Code)
	if e.Reason != "" {
		s += " (" + e.Reason + ")"
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

func (e *Error) Unwrap() error { return e.kind }

// Plan is what an account may do right now.
type Plan struct {
	Type          string     `json:"type"` // trial, starter, professional, none
	IsActive      bool       `json:"is_active"`
	TrialEndsAt   *time.Time `json:"trial_ends_at"`
	ValidUntil    *time.Time `json:"valid_until"`
	DaysRemaining *int       `json:"days_remaining"`
	Limits        struct {
		MaxPostsPerMonth  *int `json:"max_posts_per_month"`
		MaxSocialAccounts *int `json:"max_social_accounts"` // nil = no limit
	} `json:"limits"`
}

// Account is a Posta account Laamo created.
type Account struct {
	// Created and Trial are set by CreateAccount only. Trial: started (a
	// new account with its free days), used (new, but this person had a
	// trial before) or existing.
	Created bool   `json:"created"`
	Trial   string `json:"trial"`
	// APIToken is returned by CreateAccount and RotateToken only, and
	// can't be read again.
	APIToken          string `json:"api_token"`
	UserID            string `json:"user_id"`
	ExternalUserID    string `json:"external_user_id"`
	Plan              Plan   `json:"plan"`
	ConnectedAccounts int    `json:"connected_accounts"`
}

// NewAccount is what Posta needs to make an account.
type NewAccount struct {
	Email          string `json:"email"`
	ExternalUserID string `json:"external_user_id"`
	DisplayName    string `json:"display_name,omitempty"`
	// TermsAcceptedAt: when the person accepted the consent text in Laamo.
	TermsAcceptedAt time.Time `json:"terms_accepted_at"`
}

// Usage is the partner's numbers for a period (settlement).
type Usage struct {
	AccountsTotal           int `json:"accounts_total"`
	AccountsLinked          int `json:"accounts_linked"`
	AccountsCreatedInPeriod int `json:"accounts_created_in_period"`
	ActiveTrials            int `json:"active_trials"`
	ActiveStarter           int `json:"active_starter"`
	ActiveProfessional      int `json:"active_professional"`
	PostsInPeriod           int `json:"posts_in_period"`
}

// Channel is a social account connected in Posta.
type Channel struct {
	ID              string `json:"id"`
	Platform        string `json:"platform"`
	Username        string `json:"username"`
	DisplayName     string `json:"displayName"`
	ProfileImageURL string `json:"profileImageUrl"`
	IsActive        bool   `json:"isActive"`
	// ConnectionError: why the network refuses this channel, "" when fine.
	ConnectionError string `json:"connectionError,omitempty"`
}

// UnmarshalJSON reads a channel the way Posta really sends it, which is
// looser than its documentation: field names in camelCase or snake_case,
// ids as text or numbers, and null or an object where a text is described.
func (c *Channel) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	get := func(keys ...string) any {
		for _, k := range keys {
			if v, ok := m[k]; ok && v != nil {
				return v
			}
		}
		return nil
	}
	*c = Channel{
		ID:              text(get("id")),
		Platform:        text(get("platform")),
		Username:        text(get("username")),
		DisplayName:     text(get("displayName", "display_name")),
		ProfileImageURL: text(get("profileImageUrl", "profile_image_url")),
		ConnectionError: text(get("connectionError", "connection_error")),
		IsActive:        true,
	}
	if v, ok := get("isActive", "is_active").(bool); ok {
		c.IsActive = v
	}
	if c.ID == "" {
		return fmt.Errorf("a channel without an id (fields: %s)", keysOf(m))
	}
	return nil
}

// text turns a JSON value into text: numbers without a fraction as whole
// numbers, objects by their message.
func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case map[string]any:
		for _, k := range []string{"message", "error", "code"} {
			if s := text(x[k]); s != "" {
				return s
			}
		}
		return "error"
	}
	return ""
}

// keysOf lists an object's field names (never its values).
func keysOf(m map[string]any) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}

// channelList reads the answer of GET /social-accounts: the list itself,
// or an object that holds it.
func channelList(raw []byte) ([]Channel, error) {
	var list []Channel
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("neither a list nor an object")
	}
	for _, k := range []string{"accounts", "socialAccounts", "social_accounts", "data", "items"} {
		v, ok := obj[k]
		if !ok {
			continue
		}
		if err := json.Unmarshal(v, &list); err != nil {
			return nil, fmt.Errorf("%q: %w", k, err)
		}
		return list, nil
	}
	names := make(map[string]any, len(obj))
	for k := range obj {
		names[k] = nil
	}
	return nil, fmt.Errorf("no list of channels (fields: %s)", keysOf(names))
}

// caller does the HTTP for both credentials.
type caller struct {
	base  string
	token string
	http  *http.Client
}

var defaultHTTP = &http.Client{Timeout: 20 * time.Second}

func (c caller) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	base := c.base
	if base == "" {
		base = DefaultURL
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.http
	if hc == nil {
		hc = defaultHTTP
	}
	resp, err := hc.Do(req)
	if err != nil {
		// Never the request itself in the error: its header holds the key.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("posta: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("posta: %s %s: %w", method, path, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return resp.StatusCode, fmt.Errorf("posta: %s %s: answer not understood: %w", method, path, err)
			}
		}
		return resp.StatusCode, nil
	}
	return resp.StatusCode, parseError(resp.StatusCode, raw)
}

func parseError(status int, raw []byte) *Error {
	var b struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Details struct {
			Reason []string `json:"reason"`
		} `json:"details"`
	}
	_ = json.Unmarshal(raw, &b)
	e := &Error{Status: status, Code: b.Code, Message: b.Error}
	if len(b.Details.Reason) > 0 {
		e.Reason = b.Details.Reason[0]
	}
	switch {
	case status == http.StatusConflict && e.Reason == "account_exists":
		e.kind = ErrAccountExists
	case status == http.StatusConflict && e.Reason == "email_mismatch":
		e.kind = ErrEmailMismatch
	case status == http.StatusNotFound:
		e.kind = ErrNotFound
	case status == http.StatusUnauthorized:
		e.kind = ErrUnauthorized
	case status == http.StatusForbidden && e.Code == "PLAN_LIMIT_EXCEEDED":
		e.kind = ErrPlan
	}
	return e
}

// Partner is the partner API: Laamo's own accounts at Posta.
type Partner struct {
	Key  string // postapk_…
	URL  string // "" = DefaultURL
	HTTP *http.Client
}

func (p *Partner) call() caller { return caller{base: p.URL, token: p.Key, http: p.HTTP} }

// CreateAccount makes the Posta account of a Laamo user, or returns the one
// that exists. Every call returns a new token and revokes the one before:
// store the token of the latest answer, and never call this to look.
func (p *Partner) CreateAccount(ctx context.Context, a NewAccount) (Account, error) {
	var out Account
	_, err := p.call().do(ctx, http.MethodPost, "/partners/accounts", a, &out)
	if err == nil && out.APIToken == "" {
		err = errors.New("posta: account made, but no token came back")
	}
	return out, err
}

// GetAccount reads plan and channel count. ErrNotFound for an account
// Laamo didn't create (a pasted key).
func (p *Partner) GetAccount(ctx context.Context, externalUserID string) (Account, error) {
	var out Account
	_, err := p.call().do(ctx, http.MethodGet, "/partners/accounts/"+url.PathEscape(externalUserID), nil, &out)
	return out, err
}

// SetPlan sets starter, professional or none. With until, the plan ends by
// itself then unless it is set again.
func (p *Partner) SetPlan(ctx context.Context, externalUserID, plan string, until *time.Time) (Account, error) {
	in := map[string]any{"plan": plan}
	if until != nil {
		in["valid_until"] = until.UTC().Format(time.RFC3339)
	}
	var out Account
	_, err := p.call().do(ctx, http.MethodPut, "/partners/accounts/"+url.PathEscape(externalUserID)+"/plan", in, &out)
	return out, err
}

// RotateToken returns a new token and revokes the one before.
func (p *Partner) RotateToken(ctx context.Context, externalUserID string) (string, error) {
	var out struct {
		APIToken string `json:"api_token"`
	}
	_, err := p.call().do(ctx, http.MethodPost, "/partners/accounts/"+url.PathEscape(externalUserID)+"/token", nil, &out)
	if err == nil && out.APIToken == "" {
		err = errors.New("posta: no token came back")
	}
	return out.APIToken, err
}

// DeleteAccount removes the Posta account with its channels, posts and
// media. A missing account is not an error.
func (p *Partner) DeleteAccount(ctx context.Context, externalUserID string) error {
	_, err := p.call().do(ctx, http.MethodDelete, "/partners/accounts/"+url.PathEscape(externalUserID)+"?mode=delete", nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Usage returns this month's numbers.
func (p *Partner) Usage(ctx context.Context) (Usage, error) {
	var out Usage
	_, err := p.call().do(ctx, http.MethodGet, "/partners/usage", nil, &out)
	return out, err
}

// User is the normal Posta API as one user.
type User struct {
	Token string // posta_…
	URL   string
	HTTP  *http.Client
	// Poll: how long to wait between looks at a picture being processed
	// (tests shorten it; default 2s).
	Poll time.Duration
}

func (u *User) call() caller { return caller{base: u.URL, token: u.Token, http: u.HTTP} }

// Channels lists the connected social accounts. It is also how a pasted
// key is checked: ErrUnauthorized when Posta doesn't know it.
func (u *User) Channels(ctx context.Context) ([]Channel, error) {
	var raw json.RawMessage
	if _, err := u.call().do(ctx, http.MethodGet, "/social-accounts", nil, &raw); err != nil {
		return nil, err
	}
	list, err := channelList(raw)
	if err != nil {
		// Field names only: they say what Posta changed, and hold no data.
		return nil, fmt.Errorf("posta: GET /social-accounts: answer not understood: %w", err)
	}
	return list, nil
}

// Service is both sides of Posta for one deployment: the partner key, and
// user calls made with a user's token.
type Service struct {
	Partner *Partner
	// Poll is passed to every User (tests shorten it).
	Poll time.Duration
}

// New returns the service, or nil without a partner key (social posting is
// off then).
func New(partnerKey, apiURL string) *Service {
	if partnerKey == "" {
		return nil
	}
	return &Service{Partner: &Partner{Key: partnerKey, URL: apiURL}}
}

// As returns the normal API as the user with this token.
func (s *Service) As(token string) *User {
	return &User{Token: token, URL: s.Partner.URL, HTTP: s.Partner.HTTP, Poll: s.Poll}
}

// Platform is a social network Posta can connect right now.
type Platform struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IsConfigured  bool   `json:"isConfigured"`
	AuthType      string `json:"authType"` // "app_password" for Bluesky, else a redirect
	SupportsImage bool   `json:"supportsImage"`
	SupportsVideo bool   `json:"supportsVideo"`
}

// Platforms lists the networks that are switched on (no token needed).
func (s *Service) Platforms(ctx context.Context) ([]Platform, error) {
	var out struct {
		Platforms []Platform `json:"platforms"`
	}
	c := caller{base: s.Partner.URL, http: s.Partner.HTTP}
	if _, err := c.do(ctx, http.MethodGet, "/social-accounts/platforms", nil, &out); err != nil {
		return nil, err
	}
	on := out.Platforms[:0]
	for _, p := range out.Platforms {
		if p.IsConfigured {
			on = append(on, p)
		}
	}
	return on, nil
}

// StartConnect begins connecting a channel: it returns the address at the
// network to send the browser to. The browser comes back to redirectURL,
// which must be on an origin registered at Posta, with success=true or
// success=false&error=… in its query.
func (u *User) StartConnect(ctx context.Context, platform, redirectURL string) (string, error) {
	var out struct {
		AuthorizationURL string `json:"authorizationUrl"`
	}
	in := map[string]string{"platform": platform, "redirectUrl": redirectURL}
	if _, err := u.call().do(ctx, http.MethodPost, "/social-accounts/oauth/initiate", in, &out); err != nil {
		return "", err
	}
	if !strings.HasPrefix(out.AuthorizationURL, "https://") {
		return "", errors.New("posta: no address to connect at came back")
	}
	return out.AuthorizationURL, nil
}

// ConnectBluesky connects a Bluesky account with an app password (Bluesky
// has no redirect). A wrong handle or password is an *Error with status 400
// or 401 from Bluesky's side.
func (u *User) ConnectBluesky(ctx context.Context, identifier, appPassword string) error {
	in := map[string]string{"identifier": identifier, "appPassword": appPassword}
	_, err := u.call().do(ctx, http.MethodPost, "/social-accounts/bluesky/connect", in, nil)
	return err
}

// Disconnect removes a channel. A missing one is not an error.
func (u *User) Disconnect(ctx context.Context, channelID string) error {
	_, err := u.call().do(ctx, http.MethodDelete, "/social-accounts/"+url.PathEscape(channelID), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Media is a picture or video uploaded to Posta.
type Media struct {
	ID     string `json:"id"`
	Status string `json:"status"` // pending, processing, completed, failed
}

// MaxImage is the largest picture Posta takes.
const MaxImage = 20 << 20

// Upload puts a picture into the user's Posta account and waits until it
// is ready for a post (Posta makes a version for each network first). It
// returns the media id. mime is image/jpeg, image/png or image/webp.
func (u *User) Upload(ctx context.Context, name, mime string, data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxImage {
		return "", fmt.Errorf("posta: a picture must be between 1 byte and %d MB", MaxImage>>20)
	}
	var up struct {
		MediaID   string `json:"media_id"`
		UploadURL string `json:"upload_url"`
	}
	in := map[string]any{"name": name, "mime_type": mime, "size_bytes": len(data)}
	if _, err := u.call().do(ctx, http.MethodPost, "/media/create-upload-url", in, &up); err != nil {
		return "", err
	}
	// An https address, or (tests) one at the API's own address.
	usable := strings.HasPrefix(up.UploadURL, "https://") || (u.URL != "" && strings.HasPrefix(up.UploadURL, u.URL))
	if up.MediaID == "" || !usable {
		return "", errors.New("posta: no upload address came back")
	}
	// The bytes go straight to Posta's storage, at an address signed for
	// this one file: no token on this request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, up.UploadURL, bytes.NewReader(data))
	if err != nil {
		return "", errors.New("posta: the upload address isn't one")
	}
	req.Header.Set("Content-Type", mime)
	hc := u.HTTP
	if hc == nil {
		hc = uploadHTTP
	}
	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // never the signed address in an error
		}
		return "", fmt.Errorf("posta: upload: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("posta: upload: storage answered %d", resp.StatusCode)
	}
	if _, err := u.call().do(ctx, http.MethodPost, "/media/"+url.PathEscape(up.MediaID)+"/confirm-upload", nil, nil); err != nil {
		return "", err
	}
	// Processing takes a few seconds for a picture.
	wait := u.Poll
	if wait == 0 {
		wait = 2 * time.Second
	}
	for range 45 {
		var m Media
		if _, err := u.call().do(ctx, http.MethodGet, "/media/"+url.PathEscape(up.MediaID), nil, &m); err != nil {
			return "", err
		}
		switch m.Status {
		case "completed":
			return up.MediaID, nil
		case "failed":
			return "", &Error{Status: http.StatusUnprocessableEntity, Code: "MEDIA_FAILED", Message: "Posta couldn't process the picture"}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", errors.New("posta: the picture is still being processed; try again in a moment")
}

var uploadHTTP = &http.Client{Timeout: 2 * time.Minute}

// NewPost is a post to make. Channels are the ids from Channels.
type NewPost struct {
	Caption  string
	MediaIDs []string
	Channels []string
	// At schedules the post; nil with Draft false means "publish when told"
	// (Publish). Draft saves it in Posta for the person to finish there.
	At    *time.Time
	Draft bool
}

// Post is a post at Posta.
type Post struct {
	ID     string `json:"id"`
	Status string `json:"status"` // draft, scheduled, processing, posted, partially_posted, failed
}

// CreatePost makes a post. Channel ids go out as numbers, the way Posta
// hands them out.
func (u *User) CreatePost(ctx context.Context, p NewPost) (Post, error) {
	ids := make([]any, 0, len(p.Channels))
	for _, c := range p.Channels {
		if n, err := strconv.ParseInt(c, 10, 64); err == nil {
			ids = append(ids, n)
		} else {
			ids = append(ids, c)
		}
	}
	media := p.MediaIDs
	if media == nil {
		media = []string{}
	}
	in := map[string]any{"caption": p.Caption, "mediaIds": media, "socialAccountIds": ids, "isDraft": p.Draft}
	if p.At != nil && !p.Draft {
		in["scheduledAt"] = p.At.UTC().Format(time.RFC3339)
	}
	var out Post
	_, err := u.call().do(ctx, http.MethodPost, "/posts", in, &out)
	if err == nil && out.ID == "" {
		err = errors.New("posta: the post was made, but no id came back")
	}
	return out, err
}

// Publish sends a post out now. Posta works through the channels in the
// background; the result comes later (GetPost, or a webhook).
func (u *User) Publish(ctx context.Context, postID string) error {
	_, err := u.call().do(ctx, http.MethodPost, "/posts/"+url.PathEscape(postID)+"/publish", nil, nil)
	return err
}

// GetPost reads a post's state.
func (u *User) GetPost(ctx context.Context, postID string) (Post, error) {
	var out Post
	_, err := u.call().do(ctx, http.MethodGet, "/posts/"+url.PathEscape(postID), nil, &out)
	return out, err
}
