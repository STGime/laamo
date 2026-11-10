// Package api is the Laamo HTTP API. Handlers implement the generated
// StrictServerInterface (gen.go) from packages/openapi/openapi.yaml.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../packages/openapi/openapi.yaml
