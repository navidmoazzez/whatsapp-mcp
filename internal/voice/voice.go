// Package voice turns text into speech for sending as a WhatsApp voice note.
//
// Optional and off by default. With no voice configured the tool that uses
// this is never registered, so a default install does not advertise something
// that can only fail.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Speaker turns text into audio bytes.
type Speaker interface {
	// Speak returns Ogg Opus audio. WhatsApp only draws the waveform player
	// for that format, so anything else would arrive as a file attachment.
	Speak(ctx context.Context, text string) ([]byte, error)
	// Name is reported by session_status.
	Name() string
}

// Config selects and configures a provider.
type Config struct {
	// Provider is "elevenlabs", or empty for off.
	Provider string
	// APIKey for the provider.
	APIKey string
	// VoiceID is which voice to speak in. For a cloned voice this is the id
	// of your own.
	VoiceID string
	// Model overrides the provider default.
	Model string
}

// New builds a Speaker, or nil when voice is off.
func New(cfg Config) (Speaker, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "off", "none":
		return nil, nil

	case "elevenlabs", "eleven":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("voice needs an API key. Set ELEVENLABS_API_KEY")
		}
		if cfg.VoiceID == "" {
			return nil, fmt.Errorf("voice needs a voice id. Set ELEVENLABS_VOICE_ID, found under Voices in your ElevenLabs dashboard")
		}
		model := cfg.Model
		if model == "" {
			// The multilingual model, so a reply can be spoken in whatever
			// language the conversation is in rather than only English.
			model = "eleven_multilingual_v2"
		}
		return &elevenLabs{apiKey: cfg.APIKey, voiceID: cfg.VoiceID, model: model}, nil
	}

	return nil, fmt.Errorf("unknown voice provider %q. Only elevenlabs is supported", cfg.Provider)
}

type elevenLabs struct {
	apiKey  string
	voiceID string
	model   string
}

func (e *elevenLabs) Name() string { return "elevenlabs/" + e.model }

var client = &http.Client{Timeout: 2 * time.Minute}

func (e *elevenLabs) Speak(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("nothing to say")
	}

	body, err := json.Marshal(map[string]any{
		"text":     text,
		"model_id": e.model,
	})
	if err != nil {
		return nil, err
	}

	// Opus at 48kHz. WhatsApp wants Ogg Opus for a playable voice note, and
	// asking the provider for it directly avoids needing ffmpeg on this path
	// at all.
	url := fmt.Sprintf(
		"https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=opus_48000_64",
		e.voiceID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/ogg")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("elevenlabs rejected the API key")
		case http.StatusNotFound:
			return nil, fmt.Errorf("elevenlabs does not know voice id %q", e.voiceID)
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("elevenlabs rate limit reached")
		case http.StatusUnprocessableEntity:
			return nil, fmt.Errorf("elevenlabs refused the request, often quota: %s", msg)
		default:
			return nil, fmt.Errorf("elevenlabs returned %d: %s", resp.StatusCode, msg)
		}
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("elevenlabs returned no audio")
	}
	return data, nil
}
