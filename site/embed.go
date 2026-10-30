// Package site holds the files gurl-server ships inside its binary: the docs
// page, its stylesheet and the install script.
package site

import "embed"

// FS is read-only and compiled into the executable, so deploying the server is
// copying one file. Nothing is looked up on disk at runtime.
//
//go:embed templates static install.sh
var FS embed.FS
