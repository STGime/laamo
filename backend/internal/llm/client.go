// Package llm is Laamo's AI brain: one OpenAI-compatible chat client (fal.ai
// OpenRouter or Mistral direct, chosen in config), JSON answers with one
// repair retry, and metering (credits, usage ledger, allowance, run caps).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Message is one chat message.
type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// Request is one chat completion.
type Request struct {
	System      string
	Messages    []Message // the user's turn(s); System goes first
	Temperature float64   // 0 = deterministic, the default for bricks
	MaxTokens   int       // 0 = DefaultMaxTokens
	JSON        bool      // ask for a JSON object answer
}

// DefaultMaxTokens caps one answer.
const DefaultMaxTokens = 1500

// Response is the model's answer and what it cost.
type Response struct {
	Text         string
	Model        string
	TokensIn     int
	TokensOut    int
	CostUSD      float64 // as reported by the provider; 0 = not reported
	FinishReason string
}

// Chatter is anything that answers a Request (the Client, or a fake).
type Chatter interface {
	Chat(ctx context.Context, req Request) (Response, error)
}

// Auth schemes for the Authorization header.
const (
	AuthKey    = "Key"    // fal.ai
	AuthBearer = "Bearer" // Mistral, OpenRouter, OpenAI-style APIs
)

// Client calls an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	BaseURL string // e.g. https://fal.run/openrouter/router/openai/v1
	APIKey  string
	Auth    string // AuthKey or AuthBearer
	Model   string
	HTTP    *http.Client
	// Retries after 429/5xx/network errors (default 2). Chat has no side
	// effects, so retrying is safe.
	Retries int
	Sleep   func(context.Context, time.Duration) error
}

// APIError is a non-2xx answer from the provider.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("llm: HTTP %d: %s", e.Status, e.Message) }

// ErrNotConfigured: no API key.
var ErrNotConfigured = errors.New("llm: no API key configured")

type chatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
	ResponseFormat any       `json:"response_format,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int      `json:"prompt_tokens"`
		CompletionTokens int      `json:"completion_tokens"`
		Cost             *float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	if c.APIKey == "" {
		return Response{}, ErrNotConfigured
	}
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, Message{Role: "system", Content: req.System})
	}
	msgs = append(msgs, req.Messages...)
	body := chatRequest{Model: c.Model, Messages: msgs, Temperature: req.Temperature, MaxTokens: req.MaxTokens}
	if body.MaxTokens <= 0 {
		body.MaxTokens = DefaultMaxTokens
	}
	if req.JSON {
		body.ResponseFormat = map[string]string{"type": "json_object"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	retries := c.Retries
	if retries == 0 {
		retries = 2
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, backoff(attempt, lastErr)); err != nil {
				return Response{}, err
			}
		}
		res, err := c.once(ctx, payload)
		if err == nil {
			return res, nil
		}
		lastErr = err
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status != http.StatusTooManyRequests && apiErr.Status < 500 {
			return Response{}, err // bad request, auth, …: retrying won't help
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
	}
	return Response{}, lastErr
}

type retryAfterError struct {
	*APIError
	wait time.Duration
}

func (c *Client) once(ctx context.Context, payload []byte) (Response, error) {
	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	auth := c.Auth
	if auth == "" {
		auth = AuthBearer
	}
	httpReq.Header.Set("Authorization", auth+" "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := hc.Do(httpReq) //nolint:gosec // G704: BaseURL is config, not user input
	if err != nil {
		return Response{}, fmt.Errorf("llm: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("llm: read: %w", err)
	}

	var cr chatResponse
	_ = json.Unmarshal(data, &cr)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if cr.Error != nil && cr.Error.Message != "" {
			msg = cr.Error.Message
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		apiErr := &APIError{Status: resp.StatusCode, Message: msg}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			return Response{}, &retryAfterError{apiErr, min(time.Duration(secs)*time.Second, 30*time.Second)}
		}
		return Response{}, apiErr
	}
	if len(cr.Choices) == 0 {
		return Response{}, &APIError{Status: resp.StatusCode, Message: "no choices in the answer"}
	}
	out := Response{Text: cr.Choices[0].Message.Content, Model: cr.Model, FinishReason: cr.Choices[0].FinishReason}
	if out.Model == "" {
		out.Model = "unknown"
	}
	if cr.Usage != nil {
		out.TokensIn, out.TokensOut = cr.Usage.PromptTokens, cr.Usage.CompletionTokens
		if cr.Usage.Cost != nil {
			out.CostUSD = *cr.Usage.Cost
		}
	}
	return out, nil
}

func (e *retryAfterError) Unwrap() error { return e.APIError }

func backoff(attempt int, last error) time.Duration {
	var ra *retryAfterError
	if errors.As(last, &ra) {
		return ra.wait
	}
	d := time.Duration(500<<min(attempt-1, 4)) * time.Millisecond
	return d/2 + rand.N(d/2+1) //nolint:gosec // jitter
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
