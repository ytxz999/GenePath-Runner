package web

import "embed"

// Static holds the whole frontend. Vendoring fonts and scripts here keeps the
// binary self-contained: `go run ./cmd/server` needs no node, no bundler and no
// network access.
//
//go:embed static
var Static embed.FS
