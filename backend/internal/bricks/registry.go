package bricks

import (
	"github.com/STGime/laamo/backend/internal/bricks/look"
	"github.com/STGime/laamo/backend/internal/bricks/think"
	"github.com/STGime/laamo/backend/internal/bricks/when"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/search"
)

// Deps are the outside services bricks use. A brick whose service is
// missing isn't registered, so its runs fail with a clear Problem card.
type Deps struct {
	Search        search.Provider
	SearchCostEUR float64
}

// All returns the executors by catalog brick type.
func All(d Deps) map[string]engine.Brick {
	m := map[string]engine.Brick{
		"when.every":      when.Every,
		"when.tap":        when.Tap,
		"think.summarize": think.Summarize,
		"think.keep":      think.Keep,
	}
	if d.Search != nil {
		m["look.search"] = look.Search{Provider: d.Search, CostEUR: d.SearchCostEUR}
	}
	return m
}
