package static

import (
	"embed"
	"path/filepath"
	"text/template"

	"github.com/benbjohnson/hashfs"
)

//go:embed all:*.css
var assetsFS embed.FS
var hashFS = hashfs.NewFS(assetsFS)

var FuncMap = template.FuncMap{
	"static": func(filename string) string {
		return filepath.Join("assets", hashFS.HashName(filename))
	},
}

var Handler = hashfs.FileServer(hashFS)
