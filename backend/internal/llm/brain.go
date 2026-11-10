package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/STGime/laamo/backend/internal/store"
)

var (
	// ErrAllowance: the workspace's included brain allowance is used up
	// this month. The run stops with a Problem card offering an upgrade,
	// waiting for next month, or (later) the user's own key.
	ErrAllowance = errors.New("brain allowance used up")
	// ErrBadOutput: the model's answer couldn't be used, even after one
	// repair attempt.
	ErrBadOutput = errors.New("the AI's answer couldn't be used")
)

// CreditEUR is what one brain credit is worth in model cost.
const CreditEUR = 0.001

// Price is a model's list price per million tokens, in USD (used when the
// provider doesn't report the cost of a call).
type Price struct{ InUSD, OutUSD float64 }

// Prices for models Laamo uses. Unknown models fall back to Fallback.
var Prices = map[string]Price{
	"mistralai/mistral-small-3.2-24b-instruct": {0.10, 0.30},
	"mistral-small-latest":                     {0.10, 0.30},
}

// Fallback is the price used for unknown models: deliberately high, so an
// unpriced model can't run cheap on paper.
var Fallback = Price{3.00, 15.00}

// Meter is what the Brain needs from a run (engine.RunContext implements it).
type Meter interface {
	WorkspaceID() string
	// Record writes a usage_ledger row and adds the cost to the run; it
	// fails with a limit error once the run is over its cost limit.
	Record(ctx context.Context, u store.UsageEntry) error
	// AddTokens counts tokens against the run's token cap.
	AddTokens(in, out int) error
}

// Brain is the included AI with metering. Bricks call Ask / AskJSON.
type Brain struct {
	Chat     Chatter
	Model    string // reported in the ledger when the provider doesn't say
	Store    store.BrainStore
	USDToEUR float64 // default 0.92
	Now      func() time.Time
}

func (b *Brain) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Ask checks the allowance, asks the model, and meters the call.
func (b *Brain) Ask(ctx context.Context, m Meter, req Request) (Response, error) {
	if b == nil || b.Chat == nil {
		return Response{}, fmt.Errorf("%w: no AI configured", ErrNotConfigured)
	}
	brain, err := b.Store.GetBrain(ctx, m.WorkspaceID())
	if err != nil {
		return Response{}, fmt.Errorf("llm: load brain: %w", err)
	}
	if brain.Mode == "included" && brain.CreditsLeft(b.now()) <= 0 {
		return Response{}, ErrAllowance
	}

	res, err := b.Chat.Chat(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if err := b.meter(ctx, m, res); err != nil {
		return res, err
	}
	return res, nil
}

func (b *Brain) meter(ctx context.Context, m Meter, res Response) error {
	model := res.Model
	if model == "" || model == "unknown" {
		model = b.Model
	}
	rate := b.USDToEUR
	if rate == 0 {
		rate = 0.92
	}
	usd := res.CostUSD
	if usd == 0 {
		p, ok := Prices[model]
		if !ok {
			p = Fallback
		}
		usd = (float64(res.TokensIn)*p.InUSD + float64(res.TokensOut)*p.OutUSD) / 1e6
	}
	eur := math.Round(usd*rate*1e6) / 1e6 // usage_ledger.est_cost_eur is numeric(12, 6)
	credits := math.Round(eur/CreditEUR*1000) / 1000

	tokErr := m.AddTokens(res.TokensIn, res.TokensOut)
	recErr := m.Record(ctx, store.UsageEntry{
		Kind: "llm", BrainMode: new("included"), Model: &model,
		Units: store.Num(res.TokensIn + res.TokensOut), BrainCredits: store.Num(credits), EstCostEUR: store.Num(eur),
	})
	// The ledger row is the record of truth; a missed counter update only
	// delays the allowance check, so its error doesn't fail the brick.
	_, _ = b.Store.AddBrainUsage(ctx, m.WorkspaceID(), credits)
	return errors.Join(tokErr, recErr)
}

// AskJSON asks for a JSON object and decodes it into out. check (optional)
// validates the decoded value. An unusable answer gets one repair attempt
// (the model sees its answer and what's wrong); then ErrBadOutput.
func (b *Brain) AskJSON(ctx context.Context, m Meter, req Request, out any, check func() error) error {
	req.JSON = true
	res, err := b.Ask(ctx, m, req)
	if err != nil {
		return err
	}
	problem := decodeJSON(res, out, check)
	if problem == nil {
		return nil
	}
	repair := req
	repair.Messages = append(append([]Message{}, req.Messages...),
		Message{Role: "assistant", Content: res.Text},
		Message{Role: "user", Content: "Your answer can't be used: " + problem.Error() +
			". Reply again with only the corrected JSON object, nothing else."})
	res, err = b.Ask(ctx, m, repair)
	if err != nil {
		return err
	}
	if problem := decodeJSON(res, out, check); problem != nil {
		return fmt.Errorf("%w: %w", ErrBadOutput, problem)
	}
	return nil
}

func decodeJSON(res Response, out any, check func() error) error {
	if res.FinishReason == "length" {
		return errors.New("it was cut off (too long); answer shorter")
	}
	text := strings.TrimSpace(res.Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("it is not valid JSON (%w)", err)
	}
	if check != nil {
		return check()
	}
	return nil
}
