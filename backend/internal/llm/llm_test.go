package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// ---- client ----

func provider(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body map[string]any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		handle(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func answer(w http.ResponseWriter, text string, cost any) {
	usage := map[string]any{"prompt_tokens": 222, "completion_tokens": 120}
	if cost != nil {
		usage["cost"] = cost
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   "mistralai/mistral-small-3.2-24b-instruct",
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": text}, "finish_reason": "stop"}},
		"usage":   usage,
	})
}

func client(url string) *llm.Client {
	return &llm.Client{BaseURL: url + "/v1", APIKey: "fal-test", Auth: llm.AuthKey, Model: "mistralai/mistral-small-3.2-24b-instruct",
		Sleep: func(context.Context, time.Duration) error { return nil }}
}

func TestClientRequestAndResponse(t *testing.T) {
	srv := provider(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		msgs, _ := body["messages"].([]any)
		first, _ := msgs[0].(map[string]any)
		rf, _ := body["response_format"].(map[string]any)
		switch {
		case r.URL.Path != "/v1/chat/completions":
			http.Error(w, "path "+r.URL.Path, http.StatusNotFound)
		case r.Header.Get("Authorization") != "Key fal-test":
			http.Error(w, "auth", http.StatusUnauthorized)
		case body["model"] != "mistralai/mistral-small-3.2-24b-instruct" || body["temperature"] != 0.0 || body["max_tokens"] != 1500.0:
			http.Error(w, fmt.Sprint("body ", body), http.StatusBadRequest)
		case len(msgs) != 2 || first["role"] != "system" || rf["type"] != "json_object":
			http.Error(w, fmt.Sprint("messages ", msgs), http.StatusBadRequest)
		default:
			answer(w, `{"ok":true}`, 0.0000582)
		}
	})
	res, err := client(srv.URL).Chat(context.Background(), llm.Request{System: "be brief", JSON: true,
		Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != `{"ok":true}` || res.TokensIn != 222 || res.TokensOut != 120 || res.CostUSD != 0.0000582 || res.FinishReason != "stop" {
		t.Fatalf("response: %+v", res)
	}
}

func TestClientRetries(t *testing.T) {
	var n atomic.Int32
	statuses := []int{429, 503}
	srv := provider(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		i := int(n.Add(1)) - 1
		if i < len(statuses) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(statuses[i])
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		answer(w, "ok", nil)
	})
	if res, err := client(srv.URL).Chat(context.Background(), llm.Request{}); err != nil || res.Text != "ok" || n.Load() != 3 {
		t.Fatalf("retries: %v %d", err, n.Load())
	}

	var bad atomic.Int32
	srv = provider(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		bad.Add(1)
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"unknown model"}}`))
	})
	_, err := client(srv.URL).Chat(context.Background(), llm.Request{})
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Message != "unknown model" || bad.Load() != 1 {
		t.Fatalf("400 is final: %v (%d calls)", err, bad.Load())
	}

	var down atomic.Int32
	srv = provider(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		down.Add(1)
		w.WriteHeader(502)
	})
	if _, err := client(srv.URL).Chat(context.Background(), llm.Request{}); err == nil || down.Load() != 3 {
		t.Fatalf("gives up after 3 tries: %v (%d)", err, down.Load())
	}

	c := client(srv.URL)
	c.APIKey = ""
	if _, err := c.Chat(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrNotConfigured) {
		t.Fatalf("no key: %v", err)
	}
}

// ---- brain ----

// script answers with the given texts in order and records requests.
type script struct {
	answers []llm.Response
	reqs    []llm.Request
}

func (s *script) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	s.reqs = append(s.reqs, req)
	if len(s.answers) == 0 {
		return llm.Response{}, errors.New("no more answers")
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return a, nil
}

func reply(text string) llm.Response {
	return llm.Response{Text: text, Model: "mistralai/mistral-small-3.2-24b-instruct", TokensIn: 1000, TokensOut: 500,
		CostUSD: 0.0005, FinishReason: "stop"}
}

type meter struct {
	ws      string
	entries []store.UsageEntry
	tokens  int
	recErr  error
}

func (m *meter) WorkspaceID() string { return m.ws }
func (m *meter) Record(_ context.Context, u store.UsageEntry) error {
	m.entries = append(m.entries, u)
	return m.recErr
}
func (m *meter) AddTokens(in, out int) error { m.tokens += in + out; return nil }

func brain(t *testing.T, s *script) (*llm.Brain, *memory.Store, *meter) {
	t.Helper()
	st := memory.New()
	w, err := st.CreateWorkspace(context.Background(), store.NewWorkspace{OwnerID: store.NewID(), Name: "w"})
	if err != nil {
		t.Fatal(err)
	}
	return &llm.Brain{Chat: s, Model: "mistralai/mistral-small-3.2-24b-instruct", Store: st, USDToEUR: 0.9}, st, &meter{ws: w.ID}
}

func TestAskMeters(t *testing.T) {
	s := &script{answers: []llm.Response{reply("hi"), {Text: "no cost reported", Model: "unknown", TokensIn: 1_000_000, TokensOut: 0}}}
	b, st, m := brain(t, s)
	ctx := context.Background()

	if _, err := b.Ask(ctx, m, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	e := m.entries[0]
	// $0.0005 × 0.9 = €0.00045 = 0.45 credits
	if e.Kind != "llm" || *e.BrainMode != "included" || float64(e.EstCostEUR) != 0.00045 || float64(e.BrainCredits) != 0.45 || e.Units != 1500 {
		t.Fatalf("ledger: %+v", e)
	}
	if m.tokens != 1500 {
		t.Fatalf("tokens: %d", m.tokens)
	}
	br, _ := st.GetBrain(ctx, m.ws)
	if float64(br.UsedCreditsThisPeriod) != 0.45 {
		t.Fatalf("allowance counter: %v", br.UsedCreditsThisPeriod)
	}

	// No cost from the provider: list price of the configured model.
	if _, err := b.Ask(ctx, m, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	// 1M input tokens × $0.10 × 0.9 = €0.09 = 90 credits
	if e := m.entries[1]; *e.Model != "mistralai/mistral-small-3.2-24b-instruct" || float64(e.BrainCredits) != 90 {
		t.Fatalf("price fallback: %+v credits %v", e, e.BrainCredits)
	}
}

func TestAllowance(t *testing.T) {
	s := &script{answers: []llm.Response{reply("hi")}}
	b, st, m := brain(t, s)
	ctx := context.Background()
	if _, err := st.AddBrainUsage(ctx, m.ws, 500); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Ask(ctx, m, llm.Request{}); !errors.Is(err, llm.ErrAllowance) {
		t.Fatalf("want ErrAllowance, got %v", err)
	}
	if len(s.reqs) != 0 {
		t.Fatal("the model was called despite the used-up allowance")
	}
	// Own-key brains have no included allowance.
	br, _ := st.GetBrain(ctx, m.ws)
	br.Mode = "byok"
	st.PutBrain(br)
	if _, err := b.Ask(ctx, m, llm.Request{}); err != nil {
		t.Fatalf("byok: %v", err)
	}
}

func TestRunLimitErrorsComeBack(t *testing.T) {
	s := &script{answers: []llm.Response{reply("hi")}}
	b, _, m := brain(t, s)
	m.recErr = errors.New("limit reached: cost")
	res, err := b.Ask(context.Background(), m, llm.Request{})
	if err == nil || res.Text != "hi" || len(m.entries) != 1 {
		t.Fatalf("a run limit must surface after the call is recorded: %v %+v", err, res)
	}
}

type scores struct {
	Cards []struct {
		ID    string  `json:"id"`
		Score float64 `json:"score"`
	} `json:"cards"`
}

func TestAskJSON(t *testing.T) {
	ctx := context.Background()
	valid := `{"cards":[{"id":"c1","score":7}]}`
	check := func(out *scores) func() error {
		return func() error {
			if len(out.Cards) == 0 {
				return errors.New("no cards")
			}
			return nil
		}
	}

	t.Run("first answer is fine", func(t *testing.T) {
		s := &script{answers: []llm.Response{reply("```json\n" + valid + "\n```")}}
		b, _, m := brain(t, s)
		var out scores
		if err := b.AskJSON(ctx, m, llm.Request{Messages: []llm.Message{{Role: "user", Content: "score"}}}, &out, check(&out)); err != nil {
			t.Fatal(err)
		}
		if out.Cards[0].Score != 7 || !s.reqs[0].JSON {
			t.Fatalf("out: %+v", out)
		}
	})
	t.Run("repaired", func(t *testing.T) {
		s := &script{answers: []llm.Response{reply("Sure! Here are the scores: c1=7"), reply(valid)}}
		b, _, m := brain(t, s)
		var out scores
		if err := b.AskJSON(ctx, m, llm.Request{Messages: []llm.Message{{Role: "user", Content: "score"}}}, &out, check(&out)); err != nil {
			t.Fatal(err)
		}
		repair := s.reqs[1].Messages
		if len(repair) != 3 || repair[1].Role != "assistant" || !strings.Contains(repair[2].Content, "not valid JSON") {
			t.Fatalf("repair request: %+v", repair)
		}
		if len(m.entries) != 2 {
			t.Fatal("both calls must be metered")
		}
	})
	t.Run("check fails, then repaired", func(t *testing.T) {
		s := &script{answers: []llm.Response{reply(`{"cards":[]}`), reply(valid)}}
		b, _, m := brain(t, s)
		var out scores
		if err := b.AskJSON(ctx, m, llm.Request{}, &out, check(&out)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s.reqs[1].Messages[1].Content, "no cards") {
			t.Fatalf("repair says why: %+v", s.reqs[1].Messages)
		}
	})
	t.Run("cut off", func(t *testing.T) {
		cut := reply(`{"cards":[{"id":"c1","sco`)
		cut.FinishReason = "length"
		s := &script{answers: []llm.Response{cut, reply(valid)}}
		b, _, m := brain(t, s)
		var out scores
		if err := b.AskJSON(ctx, m, llm.Request{}, &out, nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s.reqs[1].Messages[1].Content, "cut off") {
			t.Fatalf("repair: %+v", s.reqs[1].Messages)
		}
	})
	t.Run("still bad after repair", func(t *testing.T) {
		s := &script{answers: []llm.Response{reply("nope"), reply("still nope")}}
		b, _, m := brain(t, s)
		var out scores
		if err := b.AskJSON(ctx, m, llm.Request{}, &out, nil); !errors.Is(err, llm.ErrBadOutput) {
			t.Fatalf("want ErrBadOutput, got %v", err)
		}
		if len(s.reqs) != 2 {
			t.Fatalf("exactly one repair: %d calls", len(s.reqs))
		}
	})
}

func decode(text string, out any) error {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(text), "```json"), "```"))
	return json.Unmarshal([]byte(text), out)
}
