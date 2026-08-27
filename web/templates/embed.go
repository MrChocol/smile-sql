// Package templates embeds HTML template files at compile time so the
// final binary is self-contained for offline deployment.
package templates

import "embed"

// Files is an embed.FS containing every *.html template in this directory.
//
//go:embed *.html
var Files embed.FS
