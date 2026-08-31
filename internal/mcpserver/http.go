package mcpserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewToken makes a bearer token for the HTTP transport.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "wamcp_" + hex.EncodeToString(b), nil
}

// HTTPServer serves MCP over streamable HTTP, for clients that cannot launch a
// local command. claude.ai and ChatGPT connect from their own servers rather
// than from your browser, so they need a URL rather than a binary.
//
// A token is mandatory. There is no way to start this without one, because an
// open endpoint here is an unauthenticated door into someone's entire WhatsApp
// history.
// publicHost, when non-empty, is the hostname this server is reached at from
// outside, for example whatsapp.example.com. It is required whenever a reverse
// proxy sits in front, and it is what makes that safe.
func HTTPServer(srv *mcp.Server, addr, token, publicHost string) (*http.Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("refusing to serve HTTP without a token")
	}

	// The SDK rejects any non-loopback Host header when listening on loopback.
	// That is DNS rebinding protection and it is right by default, but it also
	// blocks the correct setup of a reverse proxy terminating TLS on a real
	// hostname and forwarding to 127.0.0.1.
	//
	// Rather than switch the protection off, it is replaced below with a
	// stricter check: the Host must equal exactly the one host we were told to
	// answer on. The SDK's version allows any loopback name; this allows one
	// name and nothing else.
	opts := &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: publicHost != "",
		MaxRequestBodyBytes:        4 << 20,
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, opts,
	)

	mux := http.NewServeMux()

	// A health check that needs no token, so a tunnel or proxy can be tested
	// without handing the token to anything. It reveals nothing.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.Handle("/", requireHost(publicHost, requireToken(token, handler)))

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, nil
}

// requireToken checks the bearer token in constant time, so a wrong guess
// cannot be narrowed down by timing the reply.
func requireToken(token string, next http.Handler) http.Handler {
	want := []byte(token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The scheme is required. TrimPrefix does nothing when the prefix is
		// absent, so trimming alone would accept a bare token with no scheme
		// at all. Cut reports whether it was actually there.
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		got := strings.TrimSpace(raw)

		if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="whatsapp-mcp"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"Missing or invalid bearer token"}}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireHost rejects any request whose Host header is not the one this server
// was told to answer on.
//
// When a reverse proxy terminates TLS and forwards to loopback, the SDK's own
// DNS rebinding protection has to be turned off, because it refuses every
// non-loopback Host. This puts something stricter in its place: exactly one
// accepted hostname rather than any loopback name.
//
// An empty want means no proxy is in front, so the SDK's protection is still
// active and this is a no-op.
func requireHost(want string, next http.Handler) http.Handler {
	if want == "" {
		return next
	}
	want = strings.ToLower(strings.TrimSpace(want))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if i := strings.LastIndex(host, ":"); i != -1 && !strings.Contains(host[i:], "]") {
			host = host[:i] // strip the port, Host may carry one
		}
		if host != want {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
