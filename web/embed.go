package web

import "embed"

// FS is the browser client served by the Go HTTP server at "/".
//
//go:embed index.html app.js style.css
var FS embed.FS
