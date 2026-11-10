// Package openapi embeds the API contract so the Go api can serve it.
package openapi

import _ "embed"

// Spec is openapi.yaml as written.
//
//go:embed openapi.yaml
var Spec []byte
