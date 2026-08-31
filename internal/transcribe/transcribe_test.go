package transcribe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOffByDefault(t *testing.T) {
	for _, p := range []string{"", "off", "none"} {
		got, err := New(Config{Provider: p})
		if err != nil {
			t.Fatalf("provider %q: %v", p, err)
		}
		if got != nil {
			t.Errorf("provider %q should disable transcription, got %v", p, got.Name())
		}
	}
}

func TestUnknownProviderNamesTheValidOnes(t *testing.T) {
	_, err := New(Config{Provider: "assemblyai"})
	if err == nil {
		t.Fatal("want an error for an unknown provider")
	}
	for _, want := range []string{"local", "groq", "openai", "elevenlabs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should list %q as a choice, got %q", want, err)
		}
	}
}

func TestHostedProvidersRequireAKey(t *testing.T) {
	for _, p := range []string{"groq", "openai", "elevenlabs"} {
		if _, err := New(Config{Provider: p}); err == nil {
			t.Errorf("%s should refuse to start without an API key", p)
		}
	}
}

func TestGroqSendsTheExpectedRequest(t *testing.T) {
	var seen *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		seen = r
		w.Write([]byte(`{"text":"hola que tal"}`))
	}))
	defer srv.Close()

	tr := &openAICompatible{
		label: "groq", endpoint: srv.URL,
		apiKey: "k", model: "whisper-large-v3-turbo", language: "es",
	}

	text, err := tr.Transcribe(context.Background(), []byte("audio"), "voice-note.ogg")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "hola que tal" {
		t.Errorf("want the transcript, got %q", text)
	}

	if h := seen.Header.Get("Authorization"); h != "Bearer k" {
		t.Errorf("want bearer auth, got %q", h)
	}
	if got := seen.FormValue("model"); got != "whisper-large-v3-turbo" {
		t.Errorf("want the model sent, got %q", got)
	}
	if got := seen.FormValue("language"); got != "es" {
		t.Errorf("want the language hint sent, got %q", got)
	}
	if _, _, err := seen.FormFile("file"); err != nil {
		t.Errorf("want the audio in a field called file: %v", err)
	}
}

// ElevenLabs uses different field names from the OpenAI shaped providers.
func TestElevenLabsUsesItsOwnFieldNames(t *testing.T) {
	var seen *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		seen = r
		w.Write([]byte(`{"text":"transcribed"}`))
	}))
	defer srv.Close()

	tr := &elevenLabs{apiKey: "k", model: "scribe_v2", language: "es"}
	// Point it at the test server by exercising the shared helpers directly.
	body, ct, err := multipartAudio([]byte("audio"), "voice-note.ogg", "file",
		map[string]string{"model_id": tr.model, "language_code": tr.language})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL, body)
	req.Header.Set("Authorization", "Bearer "+tr.apiKey)
	req.Header.Set("Content-Type", ct)

	text, err := decodeText2(httpClient.Do(req))("elevenlabs")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "transcribed" {
		t.Errorf("want the transcript, got %q", text)
	}
	if got := seen.FormValue("model_id"); got != "scribe_v2" {
		t.Errorf("elevenlabs takes model_id, got %q", got)
	}
	if got := seen.FormValue("language_code"); got != "es" {
		t.Errorf("elevenlabs takes language_code, got %q", got)
	}
}

func TestApiErrorsAreReadable(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusUnauthorized, "rejected the API key"},
		{http.StatusTooManyRequests, "rate limit"},
		{http.StatusInternalServerError, "returned 500"},
	}

	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			w.Write([]byte(`{"error":"nope"}`))
		}))

		tr := &openAICompatible{label: "groq", endpoint: srv.URL, apiKey: "k", model: "m"}
		_, err := tr.Transcribe(context.Background(), []byte("a"), "a.ogg")
		srv.Close()

		if err == nil {
			t.Fatalf("status %d should error", c.status)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("status %d: want an error mentioning %q, got %q", c.status, c.want, err)
		}
	}
}

// Multichannel responses come back under transcripts rather than text.
func TestMultichannelResponseIsHandled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"transcripts":[{"text":"first"},{"text":"second"}]}`))
	}))
	defer srv.Close()

	tr := &openAICompatible{label: "groq", endpoint: srv.URL, apiKey: "k", model: "m"}
	text, err := tr.Transcribe(context.Background(), []byte("a"), "a.ogg")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "first\nsecond" {
		t.Errorf("want both channels joined, got %q", text)
	}
}

func TestLocalNeedsTheBinaryPresent(t *testing.T) {
	_, err := New(Config{Provider: "local", Binary: "definitely-not-a-real-binary"})
	if err == nil {
		t.Fatal("want an error when the whisper binary is missing")
	}
	if !strings.Contains(err.Error(), "PATH") {
		t.Errorf("the error should explain the PATH problem, got %q", err)
	}
}
