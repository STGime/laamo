package bricks

import (
	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/bricks/look"
	"github.com/STGime/laamo/backend/internal/bricks/think"
	"github.com/STGime/laamo/backend/internal/bricks/when"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store"
)

// Deps are the outside services bricks use. A brick whose service is
// missing isn't registered, so its runs fail with a clear Problem card.
type Deps struct {
	Search        search.Provider
	SearchCostEUR float64
	// Post to chat: places and outbound messages, the master keys that
	// decrypt webhook URLs, and the webhook sender.
	Store do.ChatStore
	Keys  *crypto.Keyring
	Chat  do.ChatSender
	// Read a webpage (web.Fetcher in production).
	Pages look.PageReader
	// Find businesses (osm.Client in production).
	Businesses look.BusinessFinder
}

// All returns the executors by catalog brick type.
func All(d Deps) map[string]engine.Brick {
	m := map[string]engine.Brick{
		"when.every":      when.Every,
		"when.tap":        when.Tap,
		"think.summarize": think.Summarize,
		"think.keep":      think.Keep,
		"think.extract":   think.Extract,
		"think.write":     think.Write,
	}
	if d.Store != nil && d.Keys != nil && d.Chat != nil {
		m["do.chat"] = do.Chat{Store: d.Store, Keys: d.Keys, Sender: d.Chat}
	}
	if ls, ok := d.Store.(store.ListStore); ok {
		m["do.list"] = do.List{Store: ls}
		m["look.my_stuff"] = look.MyStuff{Lists: ls}
	}
	if d.Businesses != nil {
		m["look.find_businesses"] = look.FindBusinesses{Finder: d.Businesses}
	}
	if d.Pages != nil {
		m["look.read_page"] = look.ReadPage{Reader: d.Pages}
	}
	if d.Search != nil {
		m["look.search"] = look.Search{Provider: d.Search, CostEUR: d.SearchCostEUR}
	}
	return m
}
