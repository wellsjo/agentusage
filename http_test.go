package agentusage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticSource struct {
	snapshot Snapshot
}

func (source staticSource) Snapshot(context.Context) Snapshot {
	return source.snapshot
}

func TestHandlerServesNormalizedJSON(t *testing.T) {
	handler := Handler(staticSource{snapshot: Snapshot{Providers: []Provider{{
		ID: "codex", Name: "Codex", Windows: []Window{{ID: "primary", UsedPercent: 12}},
	}}}})
	request := httptest.NewRequest(http.MethodGet, "/ai/usage", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.HasData() {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestHandlerRejectsWrites(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(staticSource{}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/ai/usage", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
}
