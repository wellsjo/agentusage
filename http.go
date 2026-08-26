package agentusage

import (
	"encoding/json"
	"net/http"
)

// Handler returns a read-only JSON handler for a Source.
func Handler(source Source) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if source == nil {
			http.Error(w, "AI usage not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(source.Snapshot(request.Context())); err != nil {
			return
		}
	})
}
