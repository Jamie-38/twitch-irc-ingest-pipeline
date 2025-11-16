package oauth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIndexRendersAuthLink(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "abc123")
	t.Setenv("TWITCH_REDIRECT_URI", "http://localhost:3000/callback")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	Index(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "abc123") || !strings.Contains(body, "http://localhost:3000/callback") {
		t.Fatalf("body missing client/redirect: %q", body)
	}
}

func TestCallbackMissingCode(t *testing.T) {
	t.Setenv("TOKENS_PATH", os.TempDir()+"/token.json")
	req := httptest.NewRequest("GET", "/callback", nil)
	w := httptest.NewRecorder()

	Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
