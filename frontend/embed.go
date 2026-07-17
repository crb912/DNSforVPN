// Package frontend embeds the built web UI assets into the Go binary.
//
// The dist directory is produced by `npm run build` inside frontend/.
// A placeholder index.html is committed so the module compiles from a
// fresh checkout; run the frontend build before producing real binaries.
package frontend

import "embed"

// Dist holds the contents of frontend/dist (the Vite build output).
//
//go:embed all:dist
var Dist embed.FS
