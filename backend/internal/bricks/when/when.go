// Package when contains the 🟡 When bricks. They start a run: the schedule
// itself lives on the helper's trigger (internal/schedule), the Tap Go answer
// arrives with the run, so at run time these bricks only check their input
// and hand an empty pile to the next brick.
package when

import (
	"context"
	"fmt"

	"github.com/STGime/laamo/backend/internal/engine"
)

// Every… (when.every): runs on a schedule. The scheduler starts the run; the
// brick has nothing to add.
var Every = engine.BrickFunc(func(context.Context, *engine.RunContext, []engine.Card, engine.Config) ([]engine.Card, error) {
	return nil, nil
})

// Tap Go (when.tap): runs when the user taps Go. With a question, the answer
// comes with the run (trigger_data.answer) and is {answer} for later bricks.
var Tap = engine.BrickFunc(func(_ context.Context, rc *engine.RunContext, _ []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	q := cfg.String("question")
	if q == "" {
		return nil, nil
	}
	if a, ok := rc.Value("answer"); !ok || a == "" {
		return nil, fmt.Errorf("the run started without an answer to the Tap Go question “%s”: %w", q, engine.ErrMissingValue)
	}
	return nil, nil
})
