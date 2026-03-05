package static

import "embed"

// FS holds the embedded static assets (CSS, JS).
//
//go:embed css js
var FS embed.FS
