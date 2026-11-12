package bricks_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/chat"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/search"
	"github.com/STGime/laamo/backend/internal/store/memory"
	"github.com/STGime/laamo/backend/internal/web"
)

func TestRegistryMatchesCatalog(t *testing.T) {
	keys, _ := crypto.ParseKeyring("v1:"+base64.StdEncoding.EncodeToString(make([]byte, 32)), "")
	all := bricks.All(bricks.Deps{Search: dummy{}, Store: memory.New(), Keys: keys, Chat: &chat.Sender{}, Pages: &web.Fetcher{}})
	for _, want := range []string{"when.every", "when.tap", "look.search", "think.summarize", "think.keep", "do.chat", "look.read_page", "do.list", "think.extract", "think.write"} {
		if _, ok := all[want]; !ok {
			t.Errorf("%s not registered", want)
		}
	}
	for id := range all {
		b, ok := catalog.Default().Brick(id)
		if !ok {
			t.Errorf("executor for unknown brick %s", id)
		} else if b.Status != "mvp" {
			t.Errorf("executor for %s, which the catalog marks %s", id, b.Status)
		}
	}
}

type dummy struct{}

func (dummy) Search(context.Context, search.Query) ([]search.Result, error) { return nil, nil }
