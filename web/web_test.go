package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestHandlerSupportsRevalidationAndHead(t *testing.T) {
	first := httptest.NewRecorder()
	Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/agent-usage.js", nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("response has no ETag")
	}
	if got := first.Header().Get("Content-Length"); got != strconv.Itoa(first.Body.Len()) {
		t.Errorf("Content-Length = %q, body = %d bytes", got, first.Body.Len())
	}

	conditional := httptest.NewRequest(http.MethodGet, "/agent-usage.js", nil)
	conditional.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	Handler().ServeHTTP(second, conditional)
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", second.Code)
	}

	head := httptest.NewRecorder()
	Handler().ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/agent-usage.js", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD = %d with %d body bytes", head.Code, head.Body.Len())
	}
	if head.Header().Get("Content-Length") == "" {
		t.Error("HEAD response has no Content-Length")
	}
}

func TestScriptReturnsCopy(t *testing.T) {
	first := Script()
	first[0] = 0
	if second := Script(); len(second) == 0 || second[0] == 0 {
		t.Fatal("Script returned shared mutable storage")
	}
}

func TestScriptIncludesProviderIconThemes(t *testing.T) {
	script := string(Script())
	for _, marker := range []string{
		`--agentusage-icon-codex: var(--agentusage-muted)`,
		`--agentusage-icon-claude: #d97757`,
		"svg.classList.add(\"icon\", `icon--${id}`)",
	} {
		if !strings.Contains(script, marker) {
			t.Errorf("component is missing %q", marker)
		}
	}
}
