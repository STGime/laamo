package bricks_test

import (
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks"
	"github.com/STGime/laamo/backend/internal/catalog"
)

func TestRegistryMatchesCatalog(t *testing.T) {
	for id := range bricks.All() {
		b, ok := catalog.Default().Brick(id)
		if !ok {
			t.Errorf("executor for unknown brick %s", id)
		} else if b.Status != "mvp" {
			t.Errorf("executor for %s, which the catalog marks %s", id, b.Status)
		}
	}
}
