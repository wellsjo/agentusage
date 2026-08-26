package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesComponentModule(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent-usage.js", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if body := response.Body.String(); !strings.Contains(body, `customElements.define("agent-usage"`) {
		t.Fatal("response does not register agent-usage")
	}
}

func TestScriptReturnsCopy(t *testing.T) {
	first := Script()
	first[0] = 0
	if second := Script(); len(second) == 0 || second[0] == 0 {
		t.Fatal("Script returned shared mutable storage")
	}
}
