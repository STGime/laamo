// Package eurobase is a small client for the Eurobase data API
// (/v1/db/{table} REST and /v1/db/rpc/{fn}), used by store/eurobase and
// queue/eurobase. Server side only: it is meant for the secret key.
//
// Retries, by what is safe:
//   - 429: always retried after Retry-After. The request was not processed.
//   - 503: always retried. Eurobase answers 503 when nothing ran or the
//     statement was cancelled (database unavailable, SQLSTATE class 53/57).
//   - 409 with a SQLSTATE class 40 code (deadlock, serialization failure):
//     always retried; Postgres rolled the transaction back.
//   - 502, 504 and network errors: the request may or may not have run.
//     Retried only for idempotent calls (GET, PATCH, DELETE, and inserts with
//     a client-made id). Otherwise the error wraps ErrUncertain so the caller
//     can check what happened.
//   - Anything else is returned as *APIError, with the Postgres SQLSTATE in
//     Code when Eurobase sends one (RPC errors since Eurobase #829, e.g.
//     a duplicate key is 400 with code 23505; a RAISE in the function is 400
//     with its own message).
package eurobase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("eurobase: not found")
	ErrConflict  = errors.New("eurobase: conflict")
	ErrTooLarge  = errors.New("eurobase: request body over the size limit")
	ErrUncertain = errors.New("eurobase: request may or may not have been applied")
)

// MaxBody is Eurobase's request body limit (1 MB), minus headroom.
const MaxBody = 1_000_000 - 16_384

// APIError is a non-2xx answer from Eurobase.
type APIError struct {
	Method  string
	Path    string
	Status  int
	Code    string // Postgres SQLSTATE, when Eurobase sends one
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("eurobase: %s %s: %d %s (%s)", e.Method, e.Path, e.Status, e.Message, e.Code)
	}
	return fmt.Sprintf("eurobase: %s %s: %d %s", e.Method, e.Path, e.Status, e.Message)
}

// transient: Postgres rolled back a deadlock or serialization failure.
func (e *APIError) transient() bool {
	return e.Status == http.StatusConflict && strings.HasPrefix(e.Code, "40")
}

func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrConflict:
		// REST unique violation (409, no code) or a duplicate key from an RPC.
		return e.Code == "23505" || (e.Status == http.StatusConflict && !e.transient())
	case ErrTooLarge:
		return e.Status == http.StatusRequestEntityTooLarge
	}
	return false
}

type Client struct {
	base        string
	key         string
	http        *http.Client
	maxAttempts int
	maxWait     time.Duration
	sleep       func(context.Context, time.Duration) error
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithMaxAttempts sets how often a request is tried in total (default 5).
func WithMaxAttempts(n int) Option { return func(c *Client) { c.maxAttempts = max(1, n) } }

// WithSleep replaces the wait between retries (tests).
func WithSleep(f func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.sleep = f }
}

// New returns a client for baseURL (e.g. https://laamo.eurobase.app) using
// the project's secret key.
func New(baseURL, secretKey string, opts ...Option) *Client {
	c := &Client{
		base:        strings.TrimRight(baseURL, "/"),
		key:         secretKey,
		http:        &http.Client{Timeout: 30 * time.Second},
		maxAttempts: 5,
		maxWait:     30 * time.Second,
		sleep:       sleepCtx,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Get reads one row by id into out.
func (c *Client) Get(ctx context.Context, table, id string, out any) error {
	_, err := c.do(ctx, call{method: http.MethodGet, path: rowPath(table, id), idempotent: true}, out)
	return err
}

// List reads one page of rows into out (a pointer to a slice) and returns the
// total count of matching rows.
func (c *Client) List(ctx context.Context, table string, q *Query, out any) (int, error) {
	var page struct {
		Data  json.RawMessage `json:"data"`
		Count int             `json:"count"`
	}
	_, err := c.do(ctx, call{method: http.MethodGet, path: "/v1/db/" + url.PathEscape(table), query: q.values(), idempotent: true}, &page)
	if err != nil {
		return 0, err
	}
	if out != nil && len(page.Data) > 0 {
		if err := json.Unmarshal(page.Data, out); err != nil {
			return 0, fmt.Errorf("eurobase: decode %s rows: %w", table, err)
		}
	}
	return page.Count, nil
}

// ListAll reads all rows matching q, page by page (1000 per page), up to
// limit rows (0 = no limit). q's own limit and offset are replaced.
func ListAll[T any](ctx context.Context, c *Client, table string, q *Query, limit int) ([]T, error) {
	const pageSize = 1000
	if q == nil {
		q = NewQuery()
	}
	var all []T
	for offset := 0; ; offset += pageSize {
		n := pageSize
		if limit > 0 {
			n = min(pageSize, limit-len(all))
		}
		var page []T
		if _, err := c.List(ctx, table, q.clone().Limit(n).Offset(offset), &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < n || (limit > 0 && len(all) >= limit) {
			return all, nil
		}
	}
}

// Insert adds one row and decodes the stored row (with defaults) into out.
// id must be the row's own id (set by the caller): it makes the insert safe
// to retry. If a retry hits a conflict, the row from the earlier try is read
// back instead.
func (c *Client) Insert(ctx context.Context, table, id string, row, out any) error {
	if id == "" {
		return fmt.Errorf("eurobase: insert into %s: row needs a client-made id", table)
	}
	body, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("eurobase: encode %s row: %w", table, err)
	}
	retried, err := c.do(ctx, call{method: http.MethodPost, path: "/v1/db/" + url.PathEscape(table), body: body, idempotent: true}, out)
	if retried && errors.Is(err, ErrConflict) {
		// The earlier try may have gone through; it's ours if the id exists.
		if getErr := c.Get(ctx, table, id, out); getErr == nil {
			return nil
		}
	}
	return err
}

// Update patches the row with this id and decodes the new row into out.
func (c *Client) Update(ctx context.Context, table, id string, patch, out any) error {
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("eurobase: encode %s patch: %w", table, err)
	}
	_, err = c.do(ctx, call{method: http.MethodPatch, path: rowPath(table, id), body: body, idempotent: true}, out)
	return err
}

// Delete removes the row with this id. A missing row is not an error.
func (c *Client) Delete(ctx context.Context, table, id string) error {
	_, err := c.do(ctx, call{method: http.MethodDelete, path: rowPath(table, id), idempotent: true}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// RPC calls a database function. args' JSON keys are the function's named
// arguments; the function's return value is decoded into out.
// RPCs are not retried after 502/504/network errors: the error wraps
// ErrUncertain instead.
func (c *Client) RPC(ctx context.Context, fn string, args, out any) error {
	if args == nil {
		args = struct{}{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("eurobase: encode %s args: %w", fn, err)
	}
	var res struct {
		Result json.RawMessage `json:"result"`
	}
	if _, err := c.do(ctx, call{method: http.MethodPost, path: "/v1/db/rpc/" + url.PathEscape(fn), body: body}, &res); err != nil {
		return err
	}
	if out != nil && len(res.Result) > 0 {
		if err := json.Unmarshal(res.Result, out); err != nil {
			return fmt.Errorf("eurobase: decode %s result: %w", fn, err)
		}
	}
	return nil
}

// ---- transport ----

type call struct {
	method     string
	path       string
	query      url.Values
	body       []byte
	idempotent bool
}

// do runs the call with retries. retried reports whether more than one try
// was sent (an insert's conflict may then be its own earlier try).
func (c *Client) do(ctx context.Context, cl call, out any) (retried bool, err error) {
	if len(cl.body) > MaxBody {
		return false, fmt.Errorf("eurobase: %s %s: %d bytes: %w", cl.method, cl.path, len(cl.body), ErrTooLarge)
	}
	target := c.base + cl.path
	if len(cl.query) > 0 {
		target += "?" + cl.query.Encode()
	}
	for attempt := 1; ; attempt++ {
		retried = attempt > 1
		wait, err := c.try(ctx, cl, target, attempt, out)
		if wait < 0 || attempt >= c.maxAttempts {
			return retried, err
		}
		if err := c.sleep(ctx, wait); err != nil {
			return retried, err
		}
	}
}

// try sends one request. wait < 0 means done (err is the result); otherwise
// the call should be retried after wait.
func (c *Client) try(ctx context.Context, cl call, target string, attempt int, out any) (time.Duration, error) {
	var body io.Reader
	if cl.body != nil {
		body = bytes.NewReader(cl.body)
	}
	req, err := http.NewRequestWithContext(ctx, cl.method, target, body)
	if err != nil {
		return -1, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "laamo-backend")
	if cl.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return -1, ctx.Err()
		}
		err = fmt.Errorf("eurobase: %s %s: %w", cl.method, cl.path, err)
		if cl.idempotent {
			return c.backoff(attempt), err
		}
		return -1, fmt.Errorf("%w: %w", ErrUncertain, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		if cl.idempotent {
			return c.backoff(attempt), err
		}
		return -1, fmt.Errorf("%w: eurobase: read response: %w", ErrUncertain, err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && resp.StatusCode != http.StatusNoContent && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return -1, fmt.Errorf("eurobase: decode %s %s: %w", cl.method, cl.path, err)
			}
		}
		return -1, nil
	}

	msg, code := errorBody(data)
	apiErr := &APIError{Method: cl.method, Path: cl.path, Status: resp.StatusCode, Code: code, Message: msg}
	if apiErr.transient() {
		return c.backoff(attempt), apiErr
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return c.retryAfter(resp.Header.Get("Retry-After"), attempt), apiErr
	case http.StatusServiceUnavailable:
		return c.backoff(attempt), apiErr
	case http.StatusBadGateway, http.StatusGatewayTimeout:
		if cl.idempotent {
			return c.backoff(attempt), apiErr
		}
		return -1, fmt.Errorf("%w: %w", ErrUncertain, apiErr)
	}
	return -1, apiErr
}

func (c *Client) retryAfter(h string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs >= 0 {
		return min(time.Duration(secs)*time.Second, c.maxWait)
	}
	if t, err := http.ParseTime(h); err == nil {
		return min(max(time.Until(t), 0), c.maxWait)
	}
	return c.backoff(attempt)
}

// backoff doubles from 250ms up to 5s, with jitter so workers don't retry in step.
func (c *Client) backoff(attempt int) time.Duration {
	d := min(250*time.Millisecond<<min(attempt-1, 5), 5*time.Second)
	return d/2 + rand.N(d/2+1) //nolint:gosec // jitter, not security
}

func errorBody(data []byte) (msg, code string) {
	var e struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return e.Error, e.Code
	}
	if len(data) > 200 {
		data = data[:200]
	}
	return strings.TrimSpace(string(data)), ""
}

func rowPath(table, id string) string {
	return "/v1/db/" + url.PathEscape(table) + "/" + url.PathEscape(id)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
