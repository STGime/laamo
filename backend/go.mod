module github.com/STGime/laamo/backend

go 1.26

require (
	codeberg.org/readeck/go-readability/v2 v2.1.3
	github.com/STGime/laamo/packages/catalog v0.0.0
	github.com/coder/websocket v1.8.14
	github.com/temoto/robotstxt v1.1.2
	golang.org/x/net v0.57.0
)

require (
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/getkin/kin-openapi v0.142.0 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/itlightning/dateparse v0.2.1 // indirect
)

require (
	github.com/STGime/laamo/packages/openapi v0.0.0
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.2 // indirect
	github.com/oapi-codegen/runtime v1.8.0
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.24.0 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/STGime/laamo/packages/catalog => ../packages/catalog

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

replace github.com/STGime/laamo/packages/openapi => ../packages/openapi
