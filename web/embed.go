package webui

import "embed"

// Files contains the dashboard assets compiled into the single Go binary.
//
//go:embed index.html app.js style.css
var Files embed.FS
