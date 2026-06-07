package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

func webHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("embed web: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
