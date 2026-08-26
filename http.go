package agentusage

import (
	"encoding/json"
	"net/http"
	"reflect"
)

// Handler returns a read-only JSON handler for a Source.
func Handler(source Source) http.Handler {
	missing := sourceMissing(source)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if missing {
			http.Error(w, "AI usage not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if request.Method == http.MethodHead {
			return
		}
		if err := json.NewEncoder(w).Encode(source.Snapshot(request.Context())); err != nil {
			return
		}
	})
}

// sourceMissing reports a nil Source, with or without a type. A typed nil
// pointer passes a plain nil comparison and would panic on the first request.
func sourceMissing(source Source) bool {
	if source == nil {
		return true
	}
	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}
