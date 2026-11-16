package think

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/engine/tmpl"
	"github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/images"
	"github.com/STGime/laamo/backend/internal/llm"
	"github.com/STGime/laamo/backend/internal/queue"
)

// Drawer makes a picture from a prompt (images.Client).
type Drawer interface {
	Draw(ctx context.Context, prompt, shape, model string) (images.Image, error)
}

// MaxPictures: one Draw a picture brick draws at most this many per run
// (a test run: TestModeMaxCards). Other cards go on without one.
const MaxPictures = 10

// Styles: how the picture should look (for the prompt) and the model that
// draws that look best for little money (see images.Prices).
var styles = map[string]struct{ look, model string }{
	"photo":        {"a natural, realistic photo with soft daylight", "fal-ai/z-image/turbo"},
	"illustration": {"a warm, friendly digital illustration", "fal-ai/flux-2/klein/9b"},
	"flat":         {"a flat, simple vector-style graphic with few colours and clean shapes", "fal-ai/flux-2/klein/9b"},
	"watercolor":   {"a soft watercolour painting on textured paper", "fal-ai/flux-2/klein/9b"},
	"3d":           {"a cute 3D render, soft clay look, rounded shapes, studio light", "fal-ai/flux-2/klein/9b"},
	"sketch":       {"a hand-drawn pencil sketch with clean lines on white paper", "fal-ai/flux-2/klein/9b"},
}

// Draw is the Draw a picture brick (think.draw): the AI turns what a card
// says (or the user's words) into an image prompt, an image model draws it,
// and the picture is stored with the run. The card gets image (its storage
// path) and image_alt (a short description in the user's language).
type Draw struct {
	Drawer Drawer
}

func (d Draw) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 && cfg.String("mode") != "all" {
		return nil, nil
	}
	if rc.Brain == nil {
		return nil, queue.Permanent(errors.New("the AI isn't set up"))
	}
	if d.Drawer == nil || rc.Blobs == nil {
		return nil, queue.Permanent(errors.New("drawing pictures isn't set up"))
	}
	st, ok := styles[cfg.String("style")]
	if !ok {
		st = styles["illustration"]
	}
	style, model, shape := st.look, st.model, cfg.String("shape")

	if cfg.String("mode") == "all" {
		resolved, err := tmpl.Config(rc, rc.Brick, cfg, nil)
		if err != nil {
			return nil, err
		}
		var c engine.Card
		c.Kind = "image"
		if err := d.picture(ctx, rc, &c, in, resolved.String("what"), style, shape, model, "all"); err != nil {
			return nil, err
		}
		return []engine.Card{c}, nil
	}

	limit := MaxPictures
	if rc.TestMode {
		limit = engine.TestModeMaxCards
	}
	drawn, unsafe := 0, 0
	var lastErr error
	for i := range in {
		if drawn >= limit {
			rc.Log.Info("picture limit reached: the rest go on without one", "limit", limit)
			break
		}
		resolved, err := tmpl.Config(rc, rc.Brick, cfg, &in[i])
		if err != nil {
			return nil, err
		}
		err = d.picture(ctx, rc, &in[i], in[i:i+1], resolved.String("what"), style, shape, model, in[i].ID)
		switch {
		case errors.Is(err, images.ErrUnsafe) || errors.Is(err, llm.ErrBadOutput):
			unsafe++
			lastErr = err
			rc.Log.Warn("no picture for a card", "card", in[i].ID, "err", err)
		case err != nil:
			return nil, err
		default:
			drawn++
		}
	}
	if drawn == 0 && unsafe > 0 {
		return nil, lastErr
	}
	return in, nil
}

// picture writes the prompt, draws, stores the picture and sets it on c.
func (d Draw) picture(ctx context.Context, rc *engine.RunContext, c *engine.Card, about []engine.Card, what, style, shape, model, name string) error {
	if err := rc.Brain.CanSpend(ctx, rc); err != nil {
		return err
	}
	lang := language(rc, "")
	var out struct {
		Prompt string `json:"prompt"`
		Alt    string `json:"alt"`
	}
	system := fmt.Sprintf(`You write a prompt for an image generator: one picture for a small business owner, %s. The prompt is in English, at most 60 words, concrete: the subject, the setting, colours and light. Never put in: names of real or famous people, brands or logos, words, letters, numbers or signs, anything violent, scary or adult. Also give "alt": what the picture shows, at most 15 words, in %s. %s Format: {"prompt": string, "alt": string}.`,
		style, lang, guard)
	user := "Draw what these cards are about:\n" + cardsJSON(about)
	if w := strings.TrimSpace(what); w != "" {
		user = fmt.Sprintf("What to draw (the owner's words): %q\nWhat it's for:\n%s", w, cardsJSON(about))
	}
	err := rc.Brain.AskJSON(ctx, rc, llm.Request{System: system, Messages: []llm.Message{{Role: "user", Content: user}}}, &out,
		func() error {
			out.Prompt, out.Alt = strings.TrimSpace(out.Prompt), strings.TrimSpace(out.Alt)
			if out.Prompt == "" {
				return errors.New(`"prompt" is empty`)
			}
			return nil
		})
	if err != nil {
		return err
	}
	// Said here, not left to the prompt writer: image models like to add signs.
	img, err := d.Drawer.Draw(ctx, cut(out.Prompt, 800)+". Style: "+style+". No text, no letters, no logos.", shape, model)
	if err != nil {
		return err
	}
	if err := rc.Brain.Spend(ctx, rc, "image", img.Model, float64(img.Width*img.Height)/1e6, img.CostUSD); err != nil {
		return err
	}
	path := fmt.Sprintf("images/%s/%s/%s.jpg", rc.Run.WorkspaceID, cmp.Or(rc.Run.ID, "run"), name)
	if err := rc.Blobs.Put(ctx, path, img.Data); err != nil {
		// Trying again would draw (and pay) again for the same failure.
		if errors.Is(err, eurobase.ErrTooLarge) {
			return queue.Permanent(err)
		}
		return err
	}
	c.Set("image", path, engine.TrustExtracted)
	c.Set("image_alt", cut(cmp.Or(out.Alt, out.Prompt), 200), engine.TrustExtracted)
	return nil
}
