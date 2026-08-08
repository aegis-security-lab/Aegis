package webui

import (
	"embed"
	"io/fs"
)

// Files contains the production frontend copied here by the release workflow.
//
//go:embed dist
var files embed.FS

func Files() fs.FS {
	result, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return result
}
