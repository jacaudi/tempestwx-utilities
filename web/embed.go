// Package web embeds the built React UI (vendored + built into web/dist/,
// see web/PROVENANCE.md) so the Go binary can serve it without a separate
// static-file deployment step.
package web

import (
	"embed"
	"io/fs"
)

// distFS embeds the entire web/dist tree, including subdirectories, so the
// build succeeds even on a fresh checkout where dist/ holds only .gitkeep
// (see web/README.md's build-order note: a real `task ui-build` run replaces
// it with the actual Vite output before `go build`).
//
//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded UI build output, rooted at "dist" so callers
// see "index.html", "assets/...", etc. rather than "dist/index.html".
func DistFS() fs.FS {
	// "dist" is a fixed, valid path embedded via `//go:embed all:dist` above,
	// so fs.Sub cannot fail here.
	sub, _ := fs.Sub(distFS, "dist")
	return sub
}
