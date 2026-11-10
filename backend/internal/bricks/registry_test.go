package bricks_test

import (
	"context"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/search"
)

func TestRegistryMatchesCatalog(t *testing.T) {
	for id := range bricks.All(bricks.Deps{Search: dummy{}}) {
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
