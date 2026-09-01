# Working on whatsapp-mcp

For agents editing this repository. Users read the README.

## What this is

A Go binary that links to WhatsApp as a companion device, the same protocol
WhatsApp Web uses, and exposes the account over MCP. It is distributed through
npm as a platform-specific wrapper, so a reader installs it with `npx` and never
sees Go.

```
cmd/whatsapp-mcp/     entry point
internal/wa/          the WhatsApp client and media handling
internal/mcpserver/   tool registration
internal/safety/      the send guard: allowlist, rate limit, audit
internal/store/       local message history
internal/transcribe/  voice note transcription, optional
internal/agent/       the agent-facing surface
npm/                  the wrapper published to npm
```

## Non-negotiables

**Commit as `n@navid.me`.** Never pass `-c user.email=`. The global config is
correct and the override is the bug: the wrong address credits a blocked account
and the Contributors panel reads 0.

**Sending is OFF by default, and this repo is the exception to the house rule.**

Everywhere else, writes work by default, because publishing is the point of a
publishing tool. That reasoning does not transfer here:

- The point of this server is reading your own history. Sending is a secondary
  convenience, not the reason anybody installs it.
- A WhatsApp inbox is the most injectable surface in this whole family of
  servers. Every message is text a stranger chose, and a message in a group can
  instruct a model to forward private history somewhere.
- A wrong send goes to a named human being who knows you, cannot be unsent, and
  costs a relationship rather than a deleted post.
- WhatsApp bans accounts for automated behaviour. A wrong send risks the number.

So `AllowSend` must be explicitly enabled, an allowlist can narrow sends to
specific chats, sends are capped per rolling minute, and every attempt is
appended to an audit log. Do not "correct" this to match the other servers.

**The guard is the defence, not the README.** Naming prompt injection in prose
does nothing. `internal/safety/guard.go` is what actually stops it.

**Never add anything that sends in bulk or on a schedule.** Volume is what gets
a number banned, and this is somebody's personal account.

## Before claiming it works

```bash
go build ./... && go test ./...
```

A green suite is not a working server. Link a test device and run the real
handshake before saying it works.
