// Package web embeds the optional agent-usage Web Component. The component
// works in any framework and also without one.
package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	_ "embed"
)

//go:embed agent-usage.js
var component []byte

// componentETag lets clients revalidate a cached copy with a 304 response.
var componentETag = fmt.Sprintf(`"%x"`, sha256.Sum256(component))

// Script returns a copy of the Web Component ES module.
func Script() []byte {
	return bytes.Clone(component)
}

// Handler serves the Web Component ES module with cache validators.
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
		w.Header().Set("ETag", componentETag)
		http.ServeContent(w, request, "agent-usage.js", time.Time{}, bytes.NewReader(component))
	})
}
