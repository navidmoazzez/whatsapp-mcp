# Security

This software links to your WhatsApp account and keeps a copy of your message
history. That is a serious thing to run, so this page says plainly what it
does, what protects you, and what it cannot protect you from.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting: open the
[Security tab](https://github.com/navidmoazzez/whatsapp-mcp/security/advisories/new)
and click **Report a vulnerability**.

That keeps the report private until a fix exists, which a public issue does
not. Include what you found and how to reproduce it.

Please do not open a public issue for a security problem.

## The threat this is built around

Every message in a WhatsApp inbox is text written by somebody else. An agent
that can read that text and also send messages can be attacked through it:
somebody puts instructions in a group chat, your agent reads them while
summarizing, and acts on them.

This is [the lethal trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/):
private data, untrusted content, and a way out. Every WhatsApp MCP server has
this exposure. Most name it in a README and stop there.

## What actually stops it

Five defenses, each covered by tests.

**Read-only by default.** A default install physically cannot send. The tool
exists, is advertised, and refuses. Sending needs `--allow-send`, passed
deliberately.

**Preview before send.** Even with sending on, the first call returns what
would be sent without sending it. Only a second call carrying `confirm: true`
goes out, so an injected instruction has to survive you reading the preview.

**Chat allowlist.** `--send-to` limits sending to named chats. Nothing else is
reachable, whatever the model decides.

**Rate limit.** Ten sends a minute by default, so a loop cannot flood anyone.

**Audit log.** Every attempted write, allowed or refused, appends to
`audit.log`. No tool can read or edit that file, so the agent cannot cover its
tracks.

Message text returned to the model is also labeled as data rather than
instructions. That is a mitigation, not a fix.

## The network

Nothing listens by default. In stdio mode there is no port at all.

With `--http`, a bearer token is mandatory and the server refuses to start
without one. Tokens are compared in constant time, and the `Bearer` scheme is
required, so a bare token is rejected.

Behind a reverse proxy, `--public-host` restricts the server to exactly one
hostname. That replaces the SDK's DNS-rebinding protection with something
stricter: it allows one name rather than any loopback name.

`/healthz` needs no token, returns `ok`, and reveals nothing, so a tunnel can
be tested without exposing the token.

## Your data

| File | What it is |
|---|---|
| `messages.db` | Your history and its search index |
| `session.db` | Companion device keys. **These are credentials** |
| `audit.log` | Every send attempt |
| `media/` | Attachments you downloaded |

All of it under a directory created mode 700.

Nothing is uploaded by this software. No telemetry, no phone home.

Two exceptions you opt into explicitly:

**Transcription.** With a hosted provider, the audio of a voice note is
uploaded to transcribe it. Nothing else is. Use `local` and nothing leaves
your machine.

**Your agent.** Anything a tool returns enters your conversation with the
model, and that conversation goes to whoever runs it. This is true of every
MCP server, and it is the decision to understand before installing this one.

## Running on a server

`deploy/install.sh` gives the service its own user with no shell, its own 700
directory, loopback binding only, and systemd hardening: `NoNewPrivileges`,
`ProtectSystem=strict`, `ProtectHome`, one writable path, restricted address
families, and a memory cap.

The session database is a live credential for your WhatsApp account. Anyone
who reads it can act as you. Treat that box accordingly.

## The optional channel

`channel/` pushes WhatsApp messages into a running Claude Code session. Its
allowlist gates both directions. Gating only inbound would leave a hole: a
prompt injection could talk Claude into replying to somebody else.

Anyone whose chat is on that list can push text into a session that has your
files open. Add only chats you control.

## What this cannot protect you from

**Prompt injection is not solved.** Read your previews.

**WhatsApp can ban your number.** This uses the companion device protocol,
which WhatsApp does not support for automation. Sending in volume is the
fastest way to get flagged.

**A compromised machine is a compromised account.** The session keys sit on
disk, because they have to.

**Not affiliated with WhatsApp or Meta.**

## Good-faith research

Read, run and pull apart anything here. Nobody but the maintainer can change
this repository, so nothing you do while investigating puts it at risk.

The care is owed to the service the tool talks to, not to the code. When
testing, use your own account and your own data. Do not point it at somebody
else's, and do not hammer a shared API to the point where other people notice.
If a test could affect anyone but you, stop and send a private report first.

Research done in that spirit is welcome, and nothing here is a trap.
