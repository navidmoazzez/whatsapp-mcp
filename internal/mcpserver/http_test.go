package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServingHTTPWithoutATokenIsRefused(t *testing.T) {
	cs, _, _ := session(t, false)
	_ = cs

	for _, token := range []string{"", "   "} {
		if _, err := HTTPServer(nil, "127.0.0.1:0", token, ""); err == nil {
			t.Errorf("token %q should be refused", token)
		}
	}
}

func TestTokensAreLongAndPrefixed(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	b, _ := NewToken()

	if a == b {
		t.Error("tokens must not repeat")
	}
	if !strings.HasPrefix(a, "wamcp_") {
		t.Errorf("want a recognisable prefix, got %q", a)
	}
	if len(a) < 40 {
		t.Errorf("token is too short to resist guessing: %d chars", len(a))
	}
}

// The HTTP endpoint is a door into someone's entire WhatsApp history, so every
// unauthenticated shape must be refused.
func TestHTTPRejectsBadTokens(t *testing.T) {
	reached := false
	guarded := requireToken("right-token", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"wrong token", "Bearer wrong-token"},
		{"bare token, no scheme", "right-token"},
		{"empty bearer", "Bearer "},
		{"prefix of the real token", "Bearer right-tok"},
		{"real token plus suffix", "Bearer right-tokenX"},
	}

	for _, c := range cases {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d", c.name, rec.Code)
		}
		if reached {
			t.Errorf("%s: request reached the server", c.name)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: want a WWW-Authenticate header", c.name)
		}
	}
}

func TestHTTPAcceptsTheRightToken(t *testing.T) {
	reached := false
	guarded := requireToken("right-token", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer right-token")
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	if !reached || rec.Code != http.StatusOK {
		t.Errorf("a valid token should pass, got %d", rec.Code)
	}
}

// The health check must work without a token so a tunnel can be tested, and it
// must reveal nothing.
func TestHealthCheckNeedsNoTokenAndLeaksNothing(t *testing.T) {
	srv, err := HTTPServer(New(Deps{}), "127.0.0.1:0", "secret-token", "")
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 without a token, got %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "secret-token") || len(body) > 32 {
		t.Errorf("health check should reveal nothing, got %q", body)
	}
}
