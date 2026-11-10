// Package catalog embeds the brick catalog and Shelf templates, the single
// source of truth shared by the Go backend and the web builder. The Go types,
// loader and stack validator live in backend/internal/catalog.
package catalog

import "embed"

// Files holds bricks.json and templates/*.json.
//
//go:embed bricks.json templates/*.json
var Files embed.FS
