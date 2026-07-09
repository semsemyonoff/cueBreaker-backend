// Package web embeds the built frontend SPA into the cuebreaker binary.
package web

import "embed"

// Dist holds the built SPA (frontend/dist, copied to web/dist by `make
// frontend-build`). Until then it holds the placeholder index.html + a
// .gitkeep, which is enough for the embed and the backend build to succeed.
//
//go:embed all:dist
var Dist embed.FS
