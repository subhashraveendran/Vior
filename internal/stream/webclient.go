package stream

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed webclient/*
var webClientFS embed.FS

// webClientHandler returns an http.Handler that serves the embedded web client files.
func webClientHandler() http.Handler {
	sub, _ := fs.Sub(webClientFS, "webclient")
	return http.FileServer(http.FS(sub))
}
