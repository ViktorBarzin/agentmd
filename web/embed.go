// Package web holds the built browser UI, embedded into the binary.
package web

import (
	"embed"
	"io/fs"
	"testing/fstest"
)

// dist holds the UI build (dist/ui, written by `npm run build`) and a
// placeholder page that is always there, so a plain `go build` works before
// the UI is built.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built UI, or a one-page stand-in that says it is missing.
func Dist() fs.FS {
	if ui, err := fs.Sub(dist, "dist/ui"); err == nil {
		if _, err := fs.Stat(ui, "index.html"); err == nil {
			return ui
		}
	}
	page, _ := fs.ReadFile(dist, "dist/placeholder.html")
	return fstest.MapFS{"index.html": {Data: page}}
}
