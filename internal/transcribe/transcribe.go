// Package transcribe turns voice notes into searchable text.
//
// Voice notes are the biggest blind spot in a WhatsApp archive. A three minute
// message can hold the whole decision and none of it is findable. Once a
// transcript lands in the store, the FTS5 trigger indexes it automatically and
// it becomes searchable alongside typed messages.
//
// Every provider is optional and off by default. A default install transcribes
// nothing and sends no audio anywhere.
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Transcriber turns audio bytes into text.
type Transcriber interface {
	// Transcribe returns the spoken text. filename carries the extension so a
	// provider can tell what container it was given.
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
	// Name is what gets reported in session_status.
	Name() string
}

// Config selects and configures a provider.
type Config struct {
	// Provider is one of local, groq, openai, elevenlabs, or empty for off.
	Provider string
	// APIKey for the hosted providers.
	APIKey string
	// Model overrides the provider default.
	Model string
	// Language is an optional ISO-639-1 hint, for example "es". Leave empty to
	// let the provider detect it, which is what you want for mixed languages.
	Language string
	// Binary is the local whisper executable. Defaults to "whisper".
	Binary string
}

// New builds a Transcriber, or nil if transcription is off.
func New(cfg Config) (Transcriber, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "off", "none":
		return nil, nil

	case "local", "whisper":
		bin := cfg.Binary
		if bin == "" {
			bin = "whisper"
		}
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("local transcription needs %q on your PATH. Install whisper.cpp, or choose a hosted provider with --transcribe", bin)
		}
		return &localWhisper{binary: bin, model: cfg.Model, language: cfg.Language}, nil

	case "groq":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("groq transcription needs an API key. Set GROQ_API_KEY")
		}
		model := cfg.Model
		if model == "" {
			model = "whisper-large-v3-turbo"
		}
		return &openAICompatible{
			label: "groq", endpoint: "https://api.groq.com/openai/v1/audio/transcriptions",
			apiKey: cfg.APIKey, model: model, language: cfg.Language,
		}, nil

	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai transcription needs an API key. Set OPENAI_API_KEY")
		}
		model := cfg.Model
		if model == "" {
			model = "whisper-1"
		}
		return &openAICompatible{
			label: "openai", endpoint: "https://api.openai.com/v1/audio/transcriptions",
			apiKey: cfg.APIKey, model: model, language: cfg.Language,
		}, nil

	case "elevenlabs", "eleven":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("elevenlabs transcription needs an API key. Set ELEVENLABS_API_KEY")
		}
		model := cfg.Model
		if model == "" {
			model = "scribe_v2"
		}
		return &elevenLabs{apiKey: cfg.APIKey, model: model, language: cfg.Language}, nil
	}

	return nil, fmt.Errorf("unknown transcription provider %q. Choose local, groq, openai or elevenlabs", cfg.Provider)
}

// httpClient is shared. Transcription of a long voice note is slow, so the
// timeout is generous rather than the usual few seconds.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// ── Groq and OpenAI ──
//
// Groq serves an OpenAI compatible transcription endpoint, so one
// implementation covers both and only the base URL, key and model differ.

type openAICompatible struct {
	label    string
	endpoint string
	apiKey   string
	model    string
	language string
}

func (o *openAICompatible) Name() string { return o.label + "/" + o.model }

func (o *openAICompatible) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	fields := map[string]string{"model": o.model, "response_format": "json"}
	if o.language != "" {
		fields["language"] = o.language
	}

	body, contentType, err := multipartAudio(audio, filename, "file", fields)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", contentType)

	return decodeText2(httpClient.Do(req))(o.label)
}

// ── ElevenLabs ──

type elevenLabs struct {
	apiKey   string
	model    string
	language string
}

func (e *elevenLabs) Name() string { return "elevenlabs/" + e.model }

func (e *elevenLabs) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	fields := map[string]string{"model_id": e.model}
	if e.language != "" {
		fields["language_code"] = e.language
	}

	body, contentType, err := multipartAudio(audio, filename, "file", fields)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.elevenlabs.io/v1/speech-to-text", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", contentType)

	return decodeText2(httpClient.Do(req))("elevenlabs")
}

// ── Local whisper ──

type localWhisper struct {
	binary   string
	model    string
	language string
}

func (l *localWhisper) Name() string { return "local/" + l.binary }

func (l *localWhisper) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	dir, err := os.MkdirTemp("", "whatsapp-mcp-stt")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".ogg"
	}
	in := filepath.Join(dir, "audio"+ext)
	if err := os.WriteFile(in, audio, 0o600); err != nil {
		return "", err
	}

	args := []string{in, "--output_format", "txt", "--output_dir", dir}
	if l.model != "" {
		args = append(args, "--model", l.model)
	}
	if l.language != "" {
		args = append(args, "--language", l.language)
	}

	cmd := exec.CommandContext(ctx, l.binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %v: %s", l.binary, err, strings.TrimSpace(stderr.String()))
	}

	out, err := os.ReadFile(filepath.Join(dir, "audio.txt"))
	if err != nil {
		return "", fmt.Errorf("%s produced no transcript: %w", l.binary, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ── Shared helpers ──

// decodeText2 curries the (response, error) pair from Do so call sites stay one
// line each.
func decodeText2(resp *http.Response, err error) func(string) (string, error) {
	return func(label string) (string, error) { return decodeText(resp, err, label) }
}

func multipartAudio(audio []byte, filename, fileField string, fields map[string]string) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if filename == "" {
		filename = "audio.ogg"
	}
	part, err := w.CreateFormFile(fileField, filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", err
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

// decodeText reads the {"text": "..."} response every one of these providers
// returns, and turns a failure into something a person can act on.
func decodeText(resp *http.Response, err error, label string) (string, error) {
	if err != nil {
		return "", fmt.Errorf("%s request failed: %w", label, err)
	}
	if resp == nil {
		return "", fmt.Errorf("%s: no response", label)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", fmt.Errorf("%s rejected the API key (%d)", label, resp.StatusCode)
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("%s rate limit reached, try again shortly", label)
		default:
			return "", fmt.Errorf("%s returned %d: %s", label, resp.StatusCode, msg)
		}
	}

	var out struct {
		Text        string `json:"text"`
		Transcripts []struct {
			Text string `json:"text"`
		} `json:"transcripts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%s returned unreadable JSON: %w", label, err)
	}

	if out.Text == "" && len(out.Transcripts) > 0 {
		parts := make([]string, 0, len(out.Transcripts))
		for _, t := range out.Transcripts {
			if t.Text != "" {
				parts = append(parts, t.Text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n")), nil
	}
	return strings.TrimSpace(out.Text), nil
}
