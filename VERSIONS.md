# WhatsApp MCP Versions

| Component | Version | Last Updated |
|-----------|---------|--------------|
| whatsapp-mcp | 0.1.0 | 2026-08-30 |

---

## 0.1.0

First release.

### What it does

Gives any AI agent access to your personal WhatsApp. Search your whole history,
read any conversation, transcribe voice notes, and send messages, from Claude
Code, Claude Desktop, Cursor, VS Code, Windsurf, Zed, Cline, Codex CLI, Gemini
CLI, or over HTTP from Claude.ai and ChatGPT.

### Install

One binary. Go 1.26 or newer and nothing else. No Python, no uv, no ffmpeg, and
no C compiler on Windows.

```bash
go install github.com/thenavidm/whatsapp-mcp/cmd/whatsapp-mcp@latest
whatsapp-mcp login
```

### Search

- Full text search over every message, using SQLite FTS5.
- Ranked by BM25, so the best match comes first rather than the most recent.
- Highlighted snippets, so you can see why something matched.
- Accents fold, so `jose` finds `José`.
- Indexes on every filtered and sorted column.
- A real contacts table, with push names and business names resolved.

### Voice notes

- Transcribed into searchable text, indexed automatically.
- Four providers, you pick: `local` whisper, Groq, ElevenLabs, OpenAI.
- Off by default. A default install uploads nothing.
- API keys come from the environment, never from flags.
- WhatsApp voice notes are already Ogg Opus, so no conversion and no ffmpeg.

### Sending, and the safety model

- Read-only unless started with `--allow-send`.
- Previews first. Nothing sends until a second call with `confirm`.
- `--send-to` limits sending to named chats.
- Rate limited, ten a minute by default.
- Every attempt appended to an audit log the agent cannot read or edit.

### Remote access

- `--http` serves streamable HTTP for Claude.ai, ChatGPT and any client that
  needs a URL rather than a local command.
- A bearer token is mandatory. Serving without one is refused.
- Tokens are compared in constant time, and the `Bearer` scheme is required.
- Binds loopback by default, and warns when it does not.
- Unauthenticated `/healthz`, so a tunnel can be tested without exposing the
  token.

### Tools

Six, each declaring read or write annotations and an output schema:
`search_messages`, `list_messages`, `list_chats`, `search_contacts`,
`session_status`, `send_message`.

### Under the hood

- One process owns the database. No bridge, no second language, no local HTTP
  hop between components.
- Pure Go SQLite, no cgo, so it cross compiles to macOS, Linux and Windows.
- Unix integer timestamps rather than strings.
- No enforced foreign key on messages, because history sync delivers messages
  before their chats and an enforced reference drops them silently.
- The QR code draws to stderr, never stdout, because stdout carries the MCP
  protocol in a single binary.

### Tests

27 tests. They cover FTS5 availability, BM25 ordering, accent folding, FTS
syntax injection, upsert idempotency, orphan messages, index freshness, a real
MCP client and server protocol handshake, tool annotations, structured output,
every send guard, transcription provider selection and request shapes, readable
API errors, and every unauthenticated HTTP shape.

### Known limits

- Not yet verified against a live WhatsApp account.
- Sending media and voice notes is not implemented yet, only text.
- Claude.ai and ChatGPT need a public URL, so a tunnel is required.
