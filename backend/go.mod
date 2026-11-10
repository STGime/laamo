module github.com/STGime/laamo/backend

go 1.26

require (
	github.com/STGime/laamo/packages/catalog v0.0.0
	github.com/coder/websocket v1.8.14
)

replace github.com/STGime/laamo/packages/catalog => ../packages/catalog
