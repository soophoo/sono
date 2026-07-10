package web

import (
	"embed"
	"io/fs"
)

//go:embed static templates
var content embed.FS

var (
	Static    = mustSub("static")
	Templates = mustSub("templates")
)

func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(content, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
