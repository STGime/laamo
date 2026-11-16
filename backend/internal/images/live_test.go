package images

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveDraw draws one picture per model (a few cents). With
// LAAMO_IMAGES_OUT set, the pictures are written there to look at.
// task test:live.
func TestLiveDraw(t *testing.T) {
	key := os.Getenv("FAL_KEY")
	if os.Getenv("LAAMO_EUROBASE_LIVE") != "1" || key == "" {
		t.Skip("set LAAMO_EUROBASE_LIVE=1 and FAL_KEY")
	}
	looks := map[string][2]string{
		"photo":        {"fal-ai/z-image/turbo", "a natural, realistic photo with soft daylight"},
		"illustration": {"fal-ai/flux-2/klein/9b", "a warm, friendly digital illustration"},
		"flat":         {"fal-ai/flux-2/klein/9b", "a flat, simple vector-style graphic with few colours and clean shapes"},
		"watercolor":   {"fal-ai/flux-2/klein/9b", "a soft watercolour painting on textured paper"},
		"3d":           {"fal-ai/flux-2/klein/9b", "a cute 3D render, soft clay look, rounded shapes, studio light"},
		"sketch":       {"fal-ai/flux-2/klein/9b", "a hand-drawn pencil sketch with clean lines on white paper"},
	}
	c := &Client{APIKey: key}
	total := 0.0
	for name, l := range looks {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		img, err := c.Draw(ctx, "A small café table by a sunny window with a slice of cherry cake and a cup of coffee. Style: "+l[1]+". No text, no letters, no logos.", "landscape", l[0])
		cancel()
		if err != nil {
			t.Errorf("%s (%s): %v", name, l[0], err)
			continue
		}
		total += img.CostUSD
		t.Logf("%-12s %-24s %dx%d %5.1f KB $%.4f", name, l[0], img.Width, img.Height, float64(len(img.Data))/1024, img.CostUSD)
		if dir := os.Getenv("LAAMO_IMAGES_OUT"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, name+".jpg"), img.Data, 0o644)
		}
	}
	t.Logf("total $%.4f", total)
}
