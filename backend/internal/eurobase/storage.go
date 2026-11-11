package eurobase

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

func cmpOr(a, b string) string { return cmp.Or(a, b) }

// PutObject stores data under key in the project's storage (overwriting).
// It's safe to retry: the same key gets the same content.
func (c *Client) PutObject(ctx context.Context, key, contentType string, data []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("key", key); err != nil {
		return err
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, key[strings.LastIndex(key, "/")+1:]))
	h.Set("Content-Type", cmpOr(contentType, "application/octet-stream"))
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	_, err = c.do(ctx, call{method: http.MethodPost, path: "/v1/storage/upload", body: body.Bytes(),
		ctype: mw.FormDataContentType(), idempotent: true}, nil)
	return err
}

// GetObject returns the object stored under key (ErrNotFound if none).
func (c *Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	var data []byte
	_, err := c.do(ctx, call{method: http.MethodGet, path: "/v1/storage/" + escapeKey(key), idempotent: true}, &data)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return data, err
}

// DeleteObject removes the object under key; a missing object is fine.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	_, err := c.do(ctx, call{method: http.MethodDelete, path: "/v1/storage/" + escapeKey(key), idempotent: true}, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func checkKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") {
		return fmt.Errorf("eurobase: bad storage key %q", key)
	}
	return nil
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// Blobs stores large card fields in project storage under a prefix
// (engine.Blobs).
type Blobs struct {
	Client *Client
	Prefix string // e.g. "laamo/"
}

func (b Blobs) Put(ctx context.Context, path string, data []byte) error {
	return b.Client.PutObject(ctx, b.Prefix+path, "text/plain; charset=utf-8", data)
}

func (b Blobs) Get(ctx context.Context, path string) ([]byte, error) {
	return b.Client.GetObject(ctx, b.Prefix+path)
}
