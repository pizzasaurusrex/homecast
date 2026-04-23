// Package web embeds homecast's vanilla HTML/CSS/JS dashboard so the daemon
// ships as a single static binary with no separate asset deployment.
//
// The UI is plain HTML + vanilla JS by design: the install promise is
// "curl | sh on a Pi → one binary," so introducing a Node toolchain is a
// non-starter until the JS grows past a few hundred lines.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var embedded embed.FS

// FS is the UI asset tree rooted at the package's static/ directory, so
// callers see index.html at the top level rather than static/index.html.
// It is exported for tests and for slice 4 (cmd/homecast) to compose into
// a top-level mux alongside the JSON API.
var FS = mustSub(embedded, "static")

// Handler returns an http.Handler that serves the embedded UI. It delegates
// to http.FileServer, so "/" resolves to index.html and unknown paths 404.
func Handler() http.Handler {
	return http.FileServer(http.FS(FS))
}

func mustSub(root embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		// //go:embed guarantees the directory exists at build time; a failure
		// here means the build would have already broken. Panicking keeps the
		// public API (FS) as a value rather than an error-returning func.
		panic("web: sub fs " + dir + ": " + err.Error())
	}
	return sub
}
