// Package web embeds the optional framework-agnostic agent-usage Web Component.
package web

import (
	_ "embed"
	"net/http"
)

//go:embed agent-usage.js
var component []byte

// Script returns a copy of the Web Component ES module.
func Script() []byte {
	return append([]byte(nil), component...)
}

// Handler serves the Web Component ES module.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Method == http.MethodGet {
			_, _ = w.Write(component)
		}
	})
}
