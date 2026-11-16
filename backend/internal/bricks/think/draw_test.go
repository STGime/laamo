package think_test

import (
	"context"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/bricks/brickstest"
	"github.com/STGime/laamo/backend/internal/bricks/think"
	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/images"
)

type fakeDrawer struct {
	prompts, models []string
	unsafe          string // a prompt containing this is refused
}

func (f *fakeDrawer) Draw(_ context.Context, prompt, shape, model string) (images.Image, error) {
	f.prompts, f.models = append(f.prompts, prompt), append(f.models, model)
	if f.unsafe != "" && strings.Contains(prompt, f.unsafe) {
		return images.Image{}, images.ErrUnsafe
	}
	return images.Image{Data: []byte("JPEG " + shape), Width: 1024, Height: 768, Model: model, CostUSD: 0.0047}, nil
}

func TestDraw(t *testing.T) {
	m := &model{f: func(_ string, ids []string, _ int) string {
		return `{"prompt":"a cosy café table with cherry cake","alt":"Café-Tisch mit Kirschkuchen"}`
	}}
	rc := runCtx(t, "think.draw", m)
	blobs := &engine.MemoryBlobs{}
	rc.Blobs = blobs
	d := &fakeDrawer{}
	out, err := brickstest.Contract(t, "think.draw", think.Draw{Drawer: d}, rc, pile(2),
		map[string]any{"style": "photo", "shape": "square", "mode": "each"})
	if err != nil {
		t.Fatal(err)
	}
	path := out[0].Text("image")
	if len(out) != 2 || !strings.HasPrefix(path, "images/"+rc.Run.WorkspaceID+"/") || out[0].Text("image_alt") != "Café-Tisch mit Kirschkuchen" ||
		out[0].Trust["image"] != engine.TrustExtracted {
		t.Fatalf("cards %+v", out[0].Fields)
	}
	if data, _ := blobs.Get(context.Background(), path); string(data) != "JPEG square" {
		t.Fatalf("stored %q", data)
	}
	// The style picks the model; the prompt says the look and no text.
	if d.models[0] != "fal-ai/z-image/turbo" || !strings.Contains(d.prompts[0], "realistic photo") || !strings.Contains(d.prompts[0], "No text, no letters, no logos") {
		t.Fatalf("drew %v %v", d.models, d.prompts)
	}
	// The prompt writer is told the rules.
	if sys := m.reqs[0].System; !strings.Contains(sys, "Never put in") || !strings.Contains(sys, "brands or logos") {
		t.Fatalf("system: %s", sys)
	}

	// A test run draws at most 3; the rest go on without a picture.
	rc = runCtx(t, "think.draw", m)
	rc.Blobs, rc.TestMode = &engine.MemoryBlobs{}, true
	out, _ = think.Draw{Drawer: &fakeDrawer{}}.Run(context.Background(), rc, pile(5), engine.Config{"style": "flat", "shape": "landscape", "mode": "each"})
	if len(out) != 5 || out[2].Text("image") == "" || out[3].Text("image") != "" {
		t.Fatalf("test run pictures: %v / %v", out[2].Fields["image"], out[3].Fields["image"])
	}

	// One for all: a new image card.
	rc = runCtx(t, "think.draw", m)
	rc.Blobs = &engine.MemoryBlobs{}
	out, err = brickstest.Contract(t, "think.draw", think.Draw{Drawer: &fakeDrawer{}}, rc, pile(3),
		map[string]any{"style": "watercolor", "shape": "portrait", "mode": "all", "what": "a poster for our autumn menu"})
	if err != nil || len(out) != 1 || out[0].Kind != "image" || !strings.HasSuffix(out[0].Text("image"), "/all.jpg") {
		t.Fatalf("one for all: %v %+v", err, out)
	}
	if !strings.Contains(m.reqs[len(m.reqs)-1].Messages[0].Content, `"a poster for our autumn menu"`) {
		t.Fatal("the user's words weren't used")
	}
}

// A picture the safety check refuses: that card goes on without one.
func TestDrawUnsafe(t *testing.T) {
	m := &model{f: func(string, []string, int) string { return `{"prompt":"something odd","alt":"x"}` }}
	rc := runCtx(t, "think.draw", m)
	rc.Blobs = &engine.MemoryBlobs{}
	_, err := think.Draw{Drawer: &fakeDrawer{unsafe: "odd"}}.Run(context.Background(), rc, pile(2), engine.Config{"style": "photo", "mode": "each"})
	if err == nil {
		t.Fatal("all refused: no error")
	}
}
