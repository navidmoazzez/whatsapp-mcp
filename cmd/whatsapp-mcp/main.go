// Command whatsapp-mcp is a Model Context Protocol server for your personal
// WhatsApp account.
//
// One binary. It links to WhatsApp as a companion device, keeps a local
// searchable copy of your history, and serves that history to an AI agent over
// MCP. Nothing is uploaded anywhere. Messages reach a model only when the agent
// calls a tool, and only the results of that call.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/navidmoazzez/whatsapp-mcp/internal/agent"
	"github.com/navidmoazzez/whatsapp-mcp/internal/mcpserver"
	"github.com/navidmoazzez/whatsapp-mcp/internal/safety"
	"github.com/navidmoazzez/whatsapp-mcp/internal/store"
	"github.com/navidmoazzez/whatsapp-mcp/internal/transcribe"
	"github.com/navidmoazzez/whatsapp-mcp/internal/voice"
	"github.com/navidmoazzez/whatsapp-mcp/internal/wa"
)

// version is overridden at release time with -ldflags.
var version = "dev"

const usage = `whatsapp-mcp - a WhatsApp MCP server for AI agents

Usage:
  whatsapp-mcp [flags]          Link if needed, then serve MCP over stdio
  whatsapp-mcp login            Link this device and exit
  whatsapp-mcp version          Print the version

Flags:
  --data-dir PATH     Where the session and message database live
                      (default: ~/.whatsapp-mcp)
  --allow-send        Permit sending messages. Off by default, so a default
                      install can only read
  --send-to JIDS      Comma separated chat ids that may receive messages.
                      Any chat is allowed if this is not set
  --rate-limit N      Maximum sends per minute (default 10)
  --debug             Verbose WhatsApp protocol logging on stderr
  --device-name NAME  What shows under Linked Devices on your phone.
                      Default "WhatsApp MCP". Only applied when pairing, so
                      changing it means unlinking and pairing again

Remote access (for claude.ai, ChatGPT, and any client that needs a URL):
  --http ADDR         Serve MCP over HTTP instead of stdio, for example
                      127.0.0.1:8765. Binds to loopback unless you say
                      otherwise
  --token TOKEN       Bearer token clients must present. One is generated and
                      printed if you do not supply it. Serving without a token
                      is refused
  --public-host HOST  The hostname a reverse proxy reaches this on. Required
                      behind a proxy, and it is what keeps that safe: only
                      this exact Host header is answered

Transcription (optional, off by default):
  --transcribe NAME   local, groq, openai or elevenlabs. Turns voice notes
                      into searchable text
  --transcribe-model  Override the provider's default model
  --transcribe-lang   ISO-639-1 hint such as es. Omit to auto detect, which
                      is what you want for mixed languages

  API keys are read from the environment, never from flags, so they do not
  end up in your shell history or your client's config file:
    GROQ_API_KEY, OPENAI_API_KEY, ELEVENLABS_API_KEY

  local uses a whisper binary on your PATH and sends nothing anywhere.

Voice notes (optional, off by default):
  --voice elevenlabs  Speak replies in your own voice and send them as real
                      WhatsApp voice notes
  --voice-id ID       Which voice, overriding ELEVENLABS_VOICE_ID

  Reads ELEVENLABS_API_KEY and ELEVENLABS_VOICE_ID from the environment.

Auto-reply (optional, off by default):
  --agent-chat JID    The ONE chat an assistant answers by itself
  --agent-command CMD Command run for each message. The text arrives on stdin
                      and whatever it prints is sent back as the reply
  --agent-args ARGS   Arguments for that command
  --agent-dir PATH    Working directory for that command

  Example, turning a chat into a conversation with Claude Code:
    --agent-chat 4477123456@s.whatsapp.net --agent-command claude --agent-args "-p"

  Whoever can message that chat can run that command on this machine. Point it
  at a chat you control, and nothing else.

Everything is written to stderr. Stdout carries the MCP protocol and must stay
clean, so never redirect stderr into stdout.

Docs: https://github.com/navidmoazzez/whatsapp-mcp
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "whatsapp-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	defaultDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultDir = filepath.Join(home, ".whatsapp-mcp")
	}

	fs := flag.NewFlagSet("whatsapp-mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	dataDir := fs.String("data-dir", defaultDir, "where session and history live")
	allowSend := fs.Bool("allow-send", false, "permit sending messages")
	sendTo := fs.String("send-to", "", "comma separated chat ids allowed to receive")
	rateLimit := fs.Int("rate-limit", 10, "maximum sends per minute")
	debug := fs.Bool("debug", false, "verbose protocol logging")
	deviceName := fs.String("device-name", "WhatsApp MCP", "name shown under Linked Devices on your phone")
	httpAddr := fs.String("http", "", "serve MCP over HTTP at this address instead of stdio")
	httpToken := fs.String("token", "", "bearer token for the HTTP transport")
	publicHost := fs.String("public-host", "", "hostname a reverse proxy reaches this on, for example whatsapp.example.com")
	sttProvider := fs.String("transcribe", "", "local, groq, openai or elevenlabs")
	sttModel := fs.String("transcribe-model", "", "override the provider default model")
	sttLang := fs.String("transcribe-lang", "", "ISO-639-1 language hint")
	voiceProvider := fs.String("voice", "", "elevenlabs, to send voice notes spoken in your own voice")
	voiceID := fs.String("voice-id", "", "which voice to speak in, overrides ELEVENLABS_VOICE_ID")
	agentChat := fs.String("agent-chat", "", "the ONE chat an assistant answers automatically")
	agentCmd := fs.String("agent-command", "", "command to run for each message, text arrives on stdin")
	agentArgs := fs.String("agent-args", "", "space separated arguments for that command")
	agentDir := fs.String("agent-dir", "", "working directory for that command")

	args := os.Args[1:]
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch command {
	case "version":
		fmt.Println(version)
		return nil
	case "", "login", "serve":
		// handled below
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", command)
	}

	if *dataDir == "" {
		return fmt.Errorf("could not determine a home directory, pass --data-dir")
	}

	// Ctrl-C and SIGTERM unwind cleanly so the session is not left half open.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(filepath.Join(*dataDir, "messages.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	guard, err := safety.New(safety.Config{
		AllowSend: *allowSend,
		Allowlist: splitList(*sendTo),
		PerMinute: *rateLimit,
		AuditPath: filepath.Join(*dataDir, "audit.log"),
	})
	if err != nil {
		return err
	}
	defer guard.Close()

	stt, err := transcribe.New(transcribe.Config{
		Provider: *sttProvider,
		APIKey:   transcriptionKey(*sttProvider),
		Model:    *sttModel,
		Language: *sttLang,
	})
	if err != nil {
		return err
	}
	if stt != nil {
		fmt.Fprintf(os.Stderr, "Transcribing voice notes with %s.\n", stt.Name())
	}

	vid := *voiceID
	if vid == "" {
		vid = os.Getenv("ELEVENLABS_VOICE_ID")
	}
	speaker, err := voice.New(voice.Config{
		Provider: *voiceProvider,
		APIKey:   os.Getenv("ELEVENLABS_API_KEY"),
		VoiceID:  vid,
	})
	if err != nil {
		return err
	}
	if speaker != nil {
		fmt.Fprintf(os.Stderr, "Voice notes enabled via %s.\n", speaker.Name())
	}

	responder, err := agent.New(agent.Config{
		ChatJID: *agentChat,
		Command: *agentCmd,
		Args:    splitArgs(*agentArgs),
		WorkDir: *agentDir,
	})
	if err != nil {
		return err
	}
	if responder != nil {
		fmt.Fprintf(os.Stderr, "Auto-reply on: %s\n", responder.Describe())
		fmt.Fprintln(os.Stderr, "That chat can run commands on this machine. Treat it as a terminal.")
	}

	client, err := wa.New(ctx, wa.Options{
		DataDir:     *dataDir,
		Store:       st,
		Out:         os.Stderr,
		Debug:       *debug,
		Transcriber: stt,
		DeviceName:  *deviceName,
		Responder:   responder,
		Speaker:     speaker,
	})
	if err != nil {
		return err
	}
	defer client.Disconnect()

	if err := client.Connect(ctx); err != nil {
		return err
	}

	if command == "login" {
		fmt.Fprintln(os.Stderr, "Linked. You can now add this server to your AI client.")
		return nil
	}

	if guard.ReadOnly() {
		fmt.Fprintln(os.Stderr, "Running read-only. Pass --allow-send to permit sending.")
	} else {
		fmt.Fprintln(os.Stderr, "Sending is enabled. Every write is recorded in audit.log.")
	}

	mcpserver.Version = version
	srv := mcpserver.New(mcpserver.Deps{Store: st, Client: client, Guard: guard, Voice: speaker})

	if *httpAddr != "" {
		return serveHTTP(ctx, srv, *httpAddr, *httpToken, *publicHost)
	}

	fmt.Fprintln(os.Stderr, "MCP server ready on stdio.")
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// serveHTTP runs the streamable HTTP transport and shuts down cleanly on
// signal, so the WhatsApp session is not left half open.
func serveHTTP(ctx context.Context, srv *mcp.Server, addr, token, publicHost string) error {
	if strings.TrimSpace(token) == "" {
		generated, err := mcpserver.NewToken()
		if err != nil {
			return err
		}
		token = generated
		fmt.Fprintf(os.Stderr, "\nGenerated a bearer token for this session:\n\n  %s\n\nPass --token to keep the same one across restarts.\n", token)
	}

	httpSrv, err := mcpserver.HTTPServer(srv, addr, token, publicHost)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "localhost") {
		fmt.Fprintf(os.Stderr, "\nWarning: %s is not loopback, so this is reachable from your network.\n", addr)
	}
	fmt.Fprintf(os.Stderr, "MCP server ready on http://%s\n", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdown)
	}
}

// transcriptionKey reads the provider's key from the environment. Keys are
// never taken as flags, because a flag ends up in shell history and in the
// client config file that launches this process.
func transcriptionKey(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "groq":
		return os.Getenv("GROQ_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "elevenlabs", "eleven":
		return os.Getenv("ELEVENLABS_API_KEY")
	}
	return ""
}

// splitArgs splits a command's arguments on whitespace. Deliberately not a
// shell parse: no quoting, no globbing, no substitution, so nothing in a
// message can smuggle an extra argument through.
func splitArgs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
