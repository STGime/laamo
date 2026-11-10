package when_test

import (
	"errors"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/when"
	"github.com/STGime/laamo/backend/internal/engine"
)

func TestEvery(t *testing.T) {
	out, err := brickstest.Contract(t, "when.every", when.Every, nil, nil, map[string]any{"freq": "weekly", "days": []string{"mon"}})
	if err != nil || len(out) != 0 {
		t.Fatalf("every: %v %v", out, err)
	}
}

func TestTapGo(t *testing.T) {
	if out, err := brickstest.Contract(t, "when.tap", when.Tap, nil, nil, nil); err != nil || len(out) != 0 {
		t.Fatalf("no question: %v %v", out, err)
	}

	rc := brickstest.RunContext("when.tap")
	rc.SetValue("answer", "Dresden")
	if _, err := brickstest.Contract(t, "when.tap", when.Tap, rc, nil, map[string]any{"question": "Which city?"}); err != nil {
		t.Fatalf("with answer: %v", err)
	}

	_, err := brickstest.Contract(t, "when.tap", when.Tap, nil, nil, map[string]any{"question": "Which city?"})
	if !errors.Is(err, engine.ErrMissingValue) {
		t.Fatalf("question without answer: %v", err)
	}
}
