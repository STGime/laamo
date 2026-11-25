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
	"net/http"
	"net/url"
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
	ID              string  `json:"id"`
	Platform        string  `json:"platform"`
	Username        string  `json:"username"`
	DisplayName     string  `json:"displayName"`
	ProfileImageURL string  `json:"profileImageUrl"`
	IsActive        bool    `json:"isActive"`
	ConnectionError *string `json:"connectionError"`
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
}

func (u *User) call() caller { return caller{base: u.URL, token: u.Token, http: u.HTTP} }

// Channels lists the connected social accounts. It is also how a pasted
// key is checked: ErrUnauthorized when Posta doesn't know it.
func (u *User) Channels(ctx context.Context) ([]Channel, error) {
	var out struct {
		Accounts []Channel `json:"accounts"`
	}
	_, err := u.call().do(ctx, http.MethodGet, "/social-accounts", nil, &out)
	return out.Accounts, err
}

// Service is both sides of Posta for one deployment: the partner key, and
// user calls made with a user's token.
type Service struct {
	Partner *Partner
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
	return &User{Token: token, URL: s.Partner.URL, HTTP: s.Partner.HTTP}
}
