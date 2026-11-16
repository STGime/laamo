// Package images draws pictures with a fal.ai text-to-image model (FLUX
// schnell by default: about $0.003 per megapixel, ~1 s). The safety
// checker is always on; an image it flags is refused, not returned.
package images

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://fal.run"
	DefaultModel   = "fal-ai/flux/schnell"
	maxImageBytes  = 10 << 20
)

// Prices in USD per megapixel (fal.ai, checked 2026-11). All take the same
// input: prompt, image_size, num_images, enable_safety_checker,
// output_format.
var Prices = map[string]float64{
	"fal-ai/flux/schnell":    0.003, // fastest and cheapest; fine for sketches
	"fal-ai/z-image/turbo":   0.005, // strong photorealism for the price
	"fal-ai/flux-2/klein/9b": 0.006, // newer FLUX: illustration, flat, watercolour, 3D
}

// Shapes and their sizes at the model.
var Shapes = map[string]struct {
	Size          string
	Width, Height int
}{
	"square":    {"square_hd", 1024, 1024},
	"portrait":  {"portrait_4_3", 768, 1024},
	"landscape": {"landscape_4_3", 1024, 768},
}

var (
	// ErrUnsafe: the safety checker flagged the picture.
	ErrUnsafe = errors.New("images: the picture was flagged as unsafe")
	// ErrNotConfigured: no API key.
	ErrNotConfigured = errors.New("images: no fal.ai key")
)

// Image is a drawn picture.
type Image struct {
	Data    []byte
	Mime    string
	Width   int
	Height  int
	CostUSD float64
	Model   string
}

// Client calls fal.ai.
type Client struct {
	APIKey  string
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// Draw makes one picture of prompt in a shape (square, portrait, landscape)
// with a model from Prices ("" = the client's, else the default).
func (c *Client) Draw(ctx context.Context, prompt, shape, model string) (Image, error) {
	if c.APIKey == "" {
		return Image{}, ErrNotConfigured
	}
	sz, ok := Shapes[shape]
	if !ok {
		sz = Shapes["landscape"]
	}
	model = cmp.Or(model, c.Model, DefaultModel)
	price, ok := Prices[model]
	if !ok {
		return Image{}, fmt.Errorf("images: unknown model %q", model)
	}
	body, _ := json.Marshal(map[string]any{"prompt": prompt, "image_size": sz.Size, "num_images": 1,
		"enable_safety_checker": true, "output_format": "jpeg"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmp.Or(c.BaseURL, DefaultBaseURL)+"/"+model, bytes.NewReader(body))
	if err != nil {
		return Image{}, err
	}
	req.Header.Set("Authorization", "Key "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return Image{}, fmt.Errorf("images: %w", unwrapURL(err))
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return Image{}, fmt.Errorf("images: %s: %s", res.Status, strings.TrimSpace(string(raw[:min(len(raw), 200)])))
	}
	var out struct {
		Images []struct {
			URL         string `json:"url"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			ContentType string `json:"content_type"`
		} `json:"images"`
		NSFW []bool `json:"has_nsfw_concepts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Images) == 0 {
		return Image{}, fmt.Errorf("images: no picture in the answer")
	}
	if len(out.NSFW) > 0 && out.NSFW[0] {
		return Image{}, ErrUnsafe
	}
	img := out.Images[0]
	data, err := c.download(ctx, img.URL)
	if err != nil {
		return Image{}, err
	}
	w, h := cmp.Or(img.Width, sz.Width), cmp.Or(img.Height, sz.Height)
	return Image{Data: data, Mime: cmp.Or(img.ContentType, "image/jpeg"), Width: w, Height: h, Model: model,
		CostUSD: float64(w*h) / 1e6 * price}, nil
}

// download fetches the picture from fal's file store (https only).
func (c *Client) download(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("images: bad picture address")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("images: download: %w", unwrapURL(err))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("images: download: %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("images: picture too large")
	}
	return data, nil
}

// unwrapURL keeps request URLs (which can carry tokens) out of errors.
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
