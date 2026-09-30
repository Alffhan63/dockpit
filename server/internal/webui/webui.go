// Package webui embeds the built dashboard so the controller ships as a
// single binary. `make web` copies web/dist here before building.
package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
)

func init() {
	// Go's MIME table lacks these; with nosniff the browser needs them right.
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	mime.AddExtensionType(".sh", "text/x-shellscript; charset=utf-8")
}

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded UI. ok is false when the binary was built
// without it (dist holds only the placeholder).
func Handler() (h http.Handler, ok bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return http.FileServerFS(sub), true
}
