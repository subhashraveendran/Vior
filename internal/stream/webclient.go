package stream

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed webclient/*
var webClientFS embed.FS

// webClientHandler returns an http.Handler that serves the embedded web client files.
func webClientHandler() http.Handler {
	sub, err := fs.Sub(webClientFS, "webclient")
	if err != nil {
		log.Printf("stream: webclient embed missing (%v) — serving 404", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "web client not available", http.StatusNotFound)
		})
	}
	return http.FileServer(http.FS(sub))
}
