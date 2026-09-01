<div align="center">
  <img src="https://cdn.navid.media/connectors/whatsapp-icon.png" alt="WhatsApp" width="88">
</div>

# WhatsApp MCP

[![npm](https://img.shields.io/npm/v/@thenavidm/whatsapp-mcp?color=orange&label=npm)](https://www.npmjs.com/package/@thenavidm/whatsapp-mcp)
[![License](https://img.shields.io/badge/License-MIT-blue)](./LICENSE)
[![YouTube](https://img.shields.io/badge/YouTube-@thenavidm-red?logo=youtube&logoColor=white)](https://youtube.com/@thenavidm?sub_confirmation=1)
[![X](https://img.shields.io/badge/X-@thenavidm-black?logo=x)](https://x.com/thenavidm)

Give any AI agent real access to your personal WhatsApp. Search your history, read any conversation, and send messages, from Claude Code, Claude Desktop, Claude.ai, Cursor, Codex, or any MCP client.

One binary that links as a companion device, the same way WhatsApp Web does. No Python, no Docker, no second process.

Built by [Navid Moazzez](https://navid.me).

```
You: what did I promise Sarah about the deadline?

Claude: Searching your WhatsApp history.

  Sarah Chen, 14 August
  "I'll have the first draft with you by the 22nd, worst case
   the 23rd if the client review slips."

  You also mentioned it again on 19 August, confirming the 22nd.
```

## Contents

| | Section | |
|---|---|---|
| 1 | [What you can ask it](#1-what-you-can-ask-it) | Real prompts, not features |
| 2 | [Install](#2-install) | Every client, copy and paste |
| 3 | [First run](#3-first-run) | Linking your phone |
| 4 | [Tools](#4-tools) | All six, with arguments |
| 5 | [Sending safely](#5-sending-safely) | Why it cannot send by default |
| 6 | [Voice notes](#6-voice-notes) | Making them searchable |
| 7 | [How it works](#7-how-it-works) | Architecture |
| 8 | [Your data](#8-your-data) | What is stored and where |
| 9 | [Risks](#9-risks) | Read this before you install |
| 10 | [Troubleshooting](#10-troubleshooting) | When something breaks |
| 11 | [Message Claude from WhatsApp](#11-message-claude-from-whatsapp) | Optional. Your phone as the terminal |
| 12 | [Running it 24/7](#12-running-it-247) | On a server, so it never sleeps |
| 13 | [Build from source](#13-build-from-source) | Contributing |
| 14 | [FAQ](#faq-) | Common questions |

---

## 1. What you can ask it

Once connected, you talk to your own message history in plain language.

- What did I promise Sarah about the deadline?
- Summarize the 400 messages in the founders group since Tuesday.
- Find every conversation where someone sent me an invoice.
- Who have I not replied to this week?
- What was the address Tom sent me last month?
- Pull out every book recommendation anyone has ever sent me.
- Did I ever agree to a price on that project, and what was it?

The last one is the point. Full text search runs across your entire history, including transcripts of voice notes, ranked by relevance rather than by date.

---

## 2. Install

Three steps. Get the binary, link your phone, point your client at it.

### Step 1: get the binary

```bash
npx -y @thenavidm/whatsapp-mcp version
```

That downloads the prebuilt binary for your platform and runs it. Node 20 or newer.

If you would rather have it on your PATH as a normal command:

```bash
npm install -g @thenavidm/whatsapp-mcp
```

That is the whole prerequisite list. No Python, no uv, no ffmpeg, and no C
compiler on Windows.

Check it worked:

```bash
whatsapp-mcp version
```

If the command is not found, Go's bin directory is not on your PATH:

| Platform | Add this to your PATH |
|---|---|
| macOS, Linux | `$(go env GOPATH)/bin`, usually `~/go/bin` |
| Windows | `%USERPROFILE%\go\bin` |

You will need the full path to the binary for most clients below. Get it with:

| Platform | Command |
|---|---|
| macOS, Linux | `which whatsapp-mcp` |
| Windows | `where whatsapp-mcp` |

### Step 2: link your WhatsApp

```bash
whatsapp-mcp login
```

A QR code appears in the terminal. On your phone, open WhatsApp, then
**Settings**, then **Linked Devices**, then **Link a Device**, and scan it.

Do this once, before adding the server to any client. [First run](#3-first-run)
explains what happens next.

### Step 3: add it to your client

Find yours below. Add `"--allow-send"` to the arguments if you want it to send
messages as well as read them. It is read-only without that.

#### Claude Code

```bash
claude mcp add --transport stdio whatsapp -- npx -y @thenavidm/whatsapp-mcp
```

Or, if you installed the binary yourself:

```bash
claude mcp add --transport stdio whatsapp -- whatsapp-mcp
```

With sending enabled. Everything after `--` is passed to the binary untouched:

```bash
claude mcp add --transport stdio whatsapp -- whatsapp-mcp --allow-send
```

By default this applies to the current project only. To use it everywhere:

```bash
claude mcp add --scope user --transport stdio whatsapp -- whatsapp-mcp
```

Verify it:

| Command | What it does |
|---|---|
| `claude mcp list` | List every configured server |
| `claude mcp get whatsapp` | Show this server's status |
| `/mcp` | Check connection from inside a session |

#### Claude Desktop

Claude Desktop runs on macOS and Windows. There is no Linux build.

Open the Claude menu in your system menu bar, choose **Settings**, then the
**Developer** tab, then **Edit Config**. That creates the file if it does not
exist. You can also edit it directly:

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/Users/you/go/bin/whatsapp-mcp"
    }
  }
}
```

On Windows, escape the backslashes:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "C:\\Users\\you\\go\\bin\\whatsapp-mcp.exe"
    }
  }
}
```

The path must be absolute. Claude Desktop does not inherit your shell PATH, so
a bare command name will fail. Get the absolute path with `which whatsapp-mcp`
on macOS or `where whatsapp-mcp` on Windows.

To enable sending, add the flag as an argument:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/Users/you/go/bin/whatsapp-mcp",
      "args": ["--allow-send"]
    }
  }
}
```

Quit Claude Desktop completely and reopen it. Then click **Add files,
connectors, and more** at the bottom left of the message box, hover
**Connectors**, and choose **Manage connectors**. WhatsApp should be listed
with its tools.

If it is not there, the logs say why:

| Platform | Logs |
|---|---|
| macOS | `~/Library/Logs/Claude/mcp*.log` |
| Windows | `%APPDATA%\Claude\logs\mcp*.log` |

#### Cursor

| Scope | Path |
|---|---|
| All projects | `~/.cursor/mcp.json` |
| One project | `.cursor/mcp.json` in the project root |

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/full/path/to/whatsapp-mcp"
    }
  }
}
```

Restart Cursor, then check Settings, MCP. A green dot means it connected.

#### VS Code with GitHub Copilot

Create `.vscode/mcp.json` in your workspace:

```json
{
  "servers": {
    "whatsapp": {
      "type": "stdio",
      "command": "/full/path/to/whatsapp-mcp"
    }
  }
}
```

VS Code uses `servers`, not `mcpServers`. Open Copilot Chat and switch it to
Agent mode to use the tools.

#### Windsurf

| Platform | Path |
|---|---|
| macOS, Linux | `~/.codeium/windsurf/mcp_config.json` |
| Windows | `%USERPROFILE%\.codeium\windsurf\mcp_config.json` |

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/full/path/to/whatsapp-mcp"
    }
  }
}
```

#### Zed

Open Zed settings with `cmd+,` on macOS or `ctrl+,` elsewhere:

```json
{
  "context_servers": {
    "whatsapp": {
      "command": {
        "path": "/full/path/to/whatsapp-mcp",
        "args": []
      }
    }
  }
}
```

#### Cline and Roo Code

Open the MCP Servers panel, choose Configure MCP Servers, and add:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/full/path/to/whatsapp-mcp",
      "args": [],
      "disabled": false
    }
  }
}
```

#### Codex CLI

| Platform | Path |
|---|---|
| macOS, Linux | `~/.codex/config.toml` |
| Windows | `%USERPROFILE%\.codex\config.toml` |

```toml
[mcp_servers.whatsapp]
command = "/full/path/to/whatsapp-mcp"
args = []
```

#### Gemini CLI

| Platform | Path |
|---|---|
| macOS, Linux | `~/.gemini/settings.json` |
| Windows | `%USERPROFILE%\.gemini\settings.json` |

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/full/path/to/whatsapp-mcp"
    }
  }
}
```

#### Any other MCP client

The server speaks MCP over stdio. Whatever your client calls the fields, the
shape is always the same:

| Field | Value |
|---|---|
| Transport | stdio |
| Command | full path to `whatsapp-mcp` |
| Arguments | none, or `--allow-send` |
| Environment | none needed |

#### Claude.ai and ChatGPT on the web

These connect from their own servers rather than from your browser, so they need
a URL rather than a local command. Serve it over HTTP:

```bash
whatsapp-mcp --http 127.0.0.1:8765
```

A bearer token is printed on first run. Serving without one is refused, because
this endpoint reaches your entire message history. Pass `--token` to keep the
same token across restarts.

Then expose it. [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)
is free and needs no account for a quick tunnel:

```bash
cloudflared tunnel --url http://127.0.0.1:8765
```

It prints an `https://something.trycloudflare.com` URL. Check it before going
further, using the health endpoint, which needs no token:

```bash
curl https://something.trycloudflare.com/healthz
```

**In Claude.ai:** go to Settings, then Connectors, then Add custom connector.
Paste the tunnel URL. Put the bearer token in the advanced settings.

**In ChatGPT:** enable Developer mode in settings, then add the same URL as a
connector.

Two things to know. A quick tunnel URL changes every restart, so use a named
tunnel or your own host if you want it stable. And your machine has to be
awake, because the server is running on it.

---

## 3. First run

Run `whatsapp-mcp login` and a QR code is drawn in your terminal.

On your phone: WhatsApp, **Settings**, **Linked Devices**, **Link a Device**, then point the camera at the terminal.

This links a companion device, the same way WhatsApp Web does. Your phone can be asleep afterwards.

Once linked, WhatsApp starts sending your history across. This takes a few minutes and arrives in batches, so the first search may find less than the second. Ask your agent to call `session_status` to see how much has arrived.

### Naming it

By default your phone lists it as **Other device**, which is what WhatsApp shows
when a client does not identify itself. Give it a name instead:

```bash
whatsapp-mcp login --device-name "My Server"
```

That string is what appears under Linked Devices.

The icon is not ours to set. WhatsApp chooses it from a fixed list of platform
types and there is no way to send a custom logo, so it shows a desktop icon.

Names are only sent while pairing. Changing it on an existing link does nothing.
To rename, log the device out on your phone and pair again.

### Re-pairing

To unlink: on your phone, Settings, Linked Devices, tap the device, **Log out**.

Pairing again resyncs history from scratch, which is harmless but takes a few
minutes. Your stored messages are not deleted by unlinking.

The link lasts until you remove it. WhatsApp sometimes expires companion devices after a few weeks of no use, in which case run `whatsapp-mcp login` again.

### If sync stops

Your phone will say message sync is paused when the service is not running.
Start it and it resumes on its own. Nothing is lost in the meantime, WhatsApp
queues messages and delivers them on reconnect, the same as WhatsApp Web after
a few days closed.

---

## 4. Tools

Six tools. Each declares whether it reads or writes, so your client can show you the difference before anything runs.

| Tool | Reads or writes | What it does |
|---|---|---|
| `search_messages` | read | Full text search across all history, ranked by relevance |
| `list_messages` | read | Read a conversation in order, filtered by chat and date |
| `list_chats` | read | List conversations, most recent first |
| `search_contacts` | read | Find people by name or number |
| `session_status` | read | Connection health and how much history has synced |
| `send_message` | **write** | Send a message. Disabled by default |
| `download_media` | read | Fetch and decrypt an attachment, return its path |
| `send_file` | **write** | Send an image, video, document or audio file |
| `send_voice_note` | **write** | Send an Ogg Opus file as a playable voice message |

### search_messages

Full text search over message bodies and voice note transcripts. Results come back ranked by relevance using BM25, with a highlighted snippet showing the matched words in context.

| Argument | Required | Description |
|---|---|---|
| `query` | yes | Words to find. Terms are combined with AND |
| `chat_jid` | no | Restrict to a single chat |
| `limit` | no | Default 50, maximum 500 |

Accents fold, so searching `jose` finds `José`.

### list_messages

| Argument | Required | Description |
|---|---|---|
| `chat_jid` | no | Only this conversation |
| `since` | no | RFC3339 timestamp, for example `2026-08-01T00:00:00Z` |
| `until` | no | RFC3339 timestamp |
| `limit` | no | Default 50, maximum 500 |
| `offset` | no | Skip this many, for paging |

### list_chats

| Argument | Required | Description |
|---|---|---|
| `query` | no | Filter by chat name |
| `limit` | no | Default 50, maximum 500 |
| `offset` | no | Skip this many, for paging |

### search_contacts

| Argument | Required | Description |
|---|---|---|
| `query` | yes | Name or phone number, partial matches fine |
| `limit` | no | Default 50 |

Searches a real contact table, including WhatsApp push names and business names.

### session_status

No arguments. Returns whether you are linked, whether you are connected, whether history is still syncing, how many chats and messages are stored, and whether sending is on. It also returns a plain sentence explaining the current state, so the agent can tell you what to do rather than guessing.

Call this first whenever something returns nothing.

### send_message

| Argument | Required | Description |
|---|---|---|
| `chat_jid` | yes | Who to send to, from `list_chats` or `search_contacts` |
| `text` | yes | The message |
| `confirm` | no | Must be `true` to actually send. Without it you get a preview |

See the next section before turning this on.

### download_media

| Argument | Required | Description |
|---|---|---|
| `chat_jid` | yes | The chat the message is in |
| `message_id` | yes | The message carrying the attachment |

WhatsApp never delivers the file with the message, only a key and a URL, so
attachments are fetched on demand. Downloading the same one twice returns the
existing file rather than fetching it again.

### send_file

| Argument | Required | Description |
|---|---|---|
| `chat_jid` | yes | Who to send to |
| `path` | yes | Absolute path to the file |
| `caption` | no | Ignored for audio |
| `confirm` | no | Must be `true` to send. Without it you get a preview |

The type comes from the extension, so a `.jpg` arrives as a photo rather than
an attachment.

### send_voice_note

Same arguments as `send_file`. Sends an Ogg Opus file as a playable voice
message with a waveform.

WhatsApp only renders the player for Ogg Opus. Anything else arrives as a file,
so other formats are refused with the exact command to convert:

```bash
ffmpeg -i input.mp3 -c:a libopus -b:a 32k -ar 24000 -application voip out.ogg
```

ffmpeg is optional and only needed for that conversion. Nothing else here uses
it.

---

## 5. Sending safely

Every message in your WhatsApp inbox is text somebody else wrote. An agent that can read that text and also send messages can be attacked through it. Someone puts instructions in a group chat, your agent reads them while summarizing, and acts on them. This is [the lethal trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/): private data, untrusted content, and a way out.

Every other WhatsApp MCP server I could find names this risk in its README and then does nothing about it. This one has five defenses, and they are covered by tests.

**Read-only by default.** A default install physically cannot send. The tool exists, it is advertised, and it refuses. You have to add `--allow-send` deliberately.

**Preview before send.** Even with sending enabled, the first call returns what would be sent without sending it. Only a second call with `confirm: true` goes out. An injected instruction has to survive you reading the preview.

**Chat allowlist.** Limit sending to specific conversations:

```bash
whatsapp-mcp --allow-send --send-to "friend@s.whatsapp.net,team@g.us"
```

Nothing else can be written to, whatever the model decides.

**Rate limit.** Ten sends a minute by default, so a loop cannot flood anyone. Change it with `--rate-limit`.

**Audit log.** Every attempted write, allowed or refused, is appended to `~/.whatsapp-mcp/audit.log`. There is no tool that can read or edit that file, so the agent cannot cover its tracks.

Results that contain other people's words are also labeled as data rather than instructions when they are handed to the model. That is a mitigation, not a fix. Nothing here makes prompt injection impossible, and you should still read previews.

---

## 6. Voice notes

A three minute voice note can hold the whole decision, and none of it is
searchable. Turn on transcription and voice notes become text in your history,
searchable alongside everything you have typed.

This is off by default. A default install transcribes nothing and sends no
audio anywhere.

### Choose a provider

Whisper is OpenAI's speech model, and they open sourced it, so three of these
four are the same model in different places. The only real difference is whose
computer runs it.

| Provider | What it actually is | Audio leaves your machine |
|---|---|---|
| `local` | Whisper, running on your own hardware | **No** |
| `groq` | The same Whisper, on Groq's hardware. Much faster and cheaper than OpenAI | Yes |
| `openai` | The same Whisper again, on OpenAI's servers | Yes |
| `elevenlabs` | Not Whisper. A different model called Scribe, strongest across languages | Yes |

If you are unsure, start with `groq`. It is the same model as OpenAI at a
fraction of the cost and speed. Choose `local` if nothing should leave your
machine, and `elevenlabs` if your voice notes are in languages Whisper handles
poorly.

Pick one and pass it as a flag. Keys come from the environment, never from a
flag, so they stay out of your shell history and out of your client's config
file.

**Local**, nothing leaves your machine.

Needs a `whisper` command on your PATH. [whisper.cpp](https://github.com/ggerganov/whisper.cpp)
is the usual choice, and it downloads a model file once, a few hundred MB
depending on which size you pick. After that it is free and offline forever.
Slower than the hosted options on an older machine.

```bash
whatsapp-mcp --transcribe local
```

**Groq**, fastest and cheapest. Get a key at [console.groq.com](https://console.groq.com).

```bash
export GROQ_API_KEY=your_key
whatsapp-mcp --transcribe groq
```

**ElevenLabs**, best across languages. Get a key at [navid.link/elevenlabs](https://navid.link/elevenlabs).

```bash
export ELEVENLABS_API_KEY=your_key
whatsapp-mcp --transcribe elevenlabs
```

**OpenAI**. Get a key at [platform.openai.com](https://platform.openai.com).

```bash
export OPENAI_API_KEY=your_key
whatsapp-mcp --transcribe openai
```

### In a client config

Flags and environment go in the same block. Claude Desktop, for example:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/Users/you/go/bin/whatsapp-mcp",
      "args": ["--transcribe", "groq"],
      "env": {
        "GROQ_API_KEY": "your_key"
      }
    }
  }
}
```

### Options

| Flag | Default | What it does |
|---|---|---|
| `--transcribe` | off | `local`, `groq`, `elevenlabs` or `openai` |
| `--transcribe-model` | provider default | Override the model |
| `--transcribe-lang` | auto detect | ISO-639-1 hint such as `es` |

Leave the language off unless every voice note you get is in one language.
Auto detection is what makes a mixed inbox work.

### Speaking to someone in another language

Once voice notes are text, the whole loop opens up. A Spanish voice note
arrives, it transcribes, your agent reads and translates it, and you reply in
your own language. Ask for the reply in Spanish and you get that too.

### What it costs you in privacy

`local` sends nothing anywhere and keeps the promise in [Your data](#8-your-data)
intact.

The three hosted providers upload the audio of that one voice note to
transcribe it. Nothing else is uploaded, no text messages and no history. If
that is not a trade you want, use `local`.

### Notes

WhatsApp voice notes are Ogg Opus, which every provider here accepts as is, so
there is no conversion step and no ffmpeg needed.

Transcription runs in the background. A slow provider never stalls your
WhatsApp connection, and a failure is quiet: the voice note is still stored and
still readable, it just has no search text.

---

## 7. How it works

```
   Your phone
        │  WhatsApp companion device link, end to end encrypted
        ▼
┌──────────────────────────────────┐
│        whatsapp-mcp              │   one process, one binary
│                                  │
│   whatsmeow  ──►  SQLite + FTS5  │   history, indexed and searchable
│                        │         │
│                   MCP server     │   six tools, stdio
└────────────────────────┼─────────┘
                         │  JSON-RPC over stdio
                         ▼
              Claude, Cursor, ChatGPT, ...
```

It links to WhatsApp using the same companion device protocol as WhatsApp Web, through the [whatsmeow](https://github.com/tulir/whatsmeow) library. History lands in a local SQLite database with a full text index over it. The MCP server reads that database and answers your agent's questions.

One process owns the database. There is no bridge, no localhost HTTP server, and no port listening on your machine.

### Why it is fast

Search runs on a real full text index, not a scan. Every message body and voice
note transcript is mirrored into an FTS5 index, and results come back ranked by
BM25 so the best match is first rather than the most recent.

Every column that gets filtered or sorted has an index behind it, so listing a
conversation stays quick whether you have a thousand messages or a million.

Timestamps are stored as integers, so ordering is exact and there is no
timezone guesswork.

Contact names are resolved from a real contact table, including WhatsApp push
names and business names, so people show up under the name you know them by.

### Why it is safe

No port is opened on your machine. The server talks to your client over stdio
and nothing else can reach it.

Sending is off unless you turn it on, previews before it acts, obeys a chat
allowlist and a rate limit, and writes every attempt to a log the agent cannot
read or edit. [Sending safely](#5-sending-safely) covers all of it.

### Why it is small

One binary, pure Go, no cgo. That is what lets it cross compile to macOS, Linux
and Windows without a C compiler, and it is why the install is a single command
rather than a checklist.

Six tools instead of a dozen overlapping ones. Every tool definition is loaded
into your model's context at the start of every session, whether you use it or
not, so a tighter tool set leaves more room for your actual work.

---

## 8. Your data

Everything lives in `~/.whatsapp-mcp`, readable only by you.

| File | What it is |
|---|---|
| `messages.db` | Your message history and its search index |
| `session.db` | The companion device keys. These are credentials |
| `audit.log` | Every send attempt |

Nothing is uploaded anywhere by this software. There is no telemetry and no phone home.

Your messages reach an AI model only when your agent calls a tool, and only the results of that call. That is the same trust decision you make with any MCP server, and it is worth understanding before you install this one.

To wipe everything, delete the folder. You should also remove the linked device from your phone under Settings, Linked Devices.

---

## 9. Risks

Read this properly. It is short.

**This is not an official WhatsApp API.** It uses the reverse engineered companion device protocol. WhatsApp does not support it and does not have to keep it working.

**Your number could be banned.** WhatsApp bans accounts for automated behavior. Sending in volume is the fastest way to get flagged. The sending defaults here are conservative for that reason as well as for security.

**Companion device links expire.** Expect to run `whatsapp-mcp login` again occasionally.

**Prompt injection is real.** See [Sending safely](#5-sending-safely). Read previews.

**Not affiliated with WhatsApp or Meta** in any way.

---

## 10. Troubleshooting

**The command is not found after installing.** Add Go's bin directory to your PATH. It is `$(go env GOPATH)/bin` on macOS and Linux, `%USERPROFILE%\go\bin` on Windows.

**The QR code will not scan.** Make the terminal window bigger and turn the font size down until the whole code fits without wrapping. Terminals with a light background sometimes need the theme inverted.

**Device limit reached.** WhatsApp caps linked devices. Remove an old one on your phone under Settings, Linked Devices.

**Searches return nothing right after linking.** History sync takes a few minutes and arrives in batches. Ask your agent to call `session_status` to see the counts rising.

**The client shows the server as failed.** Run `whatsapp-mcp` by hand in a terminal. Startup problems print to stderr, which most clients hide. Never redirect stderr into stdout, because stdout carries the MCP protocol and anything else on it breaks the connection.

**It says sending is disabled.** That is the default. Add `--allow-send` to the arguments in your client config.

**Messages are out of sync.** Delete `~/.whatsapp-mcp/messages.db` and restart. That rebuilds history without unlinking your device. Deleting `session.db` instead forces a fresh QR pairing.

---

## 11. Message Claude from WhatsApp

Optional, and separate from everything above. Skip it if you only want Claude
to read and send WhatsApp when you ask.

Everything so far is Claude using WhatsApp when you talk to it in a terminal or
in a browser. This turns it round: you message from your phone and Claude
answers there, running on your own machine with your files open.

It is a [Claude Code channel](https://code.claude.com/docs/en/channels), which
is what makes it your Claude rather than a separate bot with its own memory. It
sees your repos, your skills and your connectors, because it runs in the
session you already have.

```
your phone  →  WhatsApp  →  this server  →  the channel, on your machine
                                                   │
                                    Claude answers, replies in the chat
```

Setup is in [channel/README.md](channel/README.md). It needs a second WhatsApp
number for Claude to be its own contact rather than you talking to yourself.

Replies can be spoken, so the answer arrives as a voice note in your own cloned
voice rather than as text.

---

## 12. Running it 24/7

Everything above runs on your own machine, which means it works while your
computer is awake and stops when it sleeps.

If you want your agent to reach your WhatsApp at any hour, from Claude.ai or
from your phone, the server has to live somewhere that stays on. Any small
Linux box does it.

### Install it as a service

```bash
# On your machine, build for the server and upload it
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o whatsapp-mcp ./cmd/whatsapp-mcp
scp whatsapp-mcp root@your-server:/usr/local/bin/whatsapp-mcp

# On the server
curl -fsSL https://raw.githubusercontent.com/navidmoazzez/whatsapp-mcp/main/deploy/install.sh | sudo bash
```

The script is deliberately tidy about sharing a box with other services:

| | |
|---|---|
| Own service user | `whatsappmcp`, no login shell, no sudo |
| Own data directory | `/var/lib/whatsapp-mcp`, mode 700 |
| Own port | 8787, and it refuses to install if something already holds it |
| Memory cap | 512MB, enforced by the kernel, so it cannot starve a neighbour |
| Binding | `127.0.0.1` only, nothing exposed until you choose to |
| Hardening | `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, one writable path |

Then link it once and start it:

```bash
sudo -u whatsappmcp /usr/local/bin/whatsapp-mcp login --data-dir /var/lib/whatsapp-mcp
sudo systemctl start whatsapp-mcp
```

### Which host

Any Linux VPS works. It needs about 200MB of RAM and a few GB of disk, so the
cheapest tier at any provider is plenty.

| Provider | Notes |
|---|---|
| [Hetzner](https://navid.link/hetzner) | Best value per euro. European data centers, plus US |
| [Hostinger](https://navid.link/hostinger) | Simple panel, good if you want less to think about |
| [DigitalOcean](https://navid.link/digitalocean) | Widest choice of regions, the best documentation |

Prices are not listed here on purpose. They move, they differ by region, and
whether VAT is included changes the headline number. Check the provider.

One thing worth knowing when you compare: the smallest tier at some providers
is 512MB of RAM, which is the same as this service's own memory cap. Take a 1GB
or 2GB tier so the operating system has room too.

It happily shares a box with something else you already run. On a server also
running a search index, this process sits at around 200MB while the neighbour
uses 80MB, on a machine with 4GB. You are unlikely to need a dedicated one.

### Reaching it

The service binds to loopback, so put your existing reverse proxy in front. A
Caddy block is two lines:

```
whatsapp.example.com {
  reverse_proxy 127.0.0.1:8787
}
```

Point DNS at the server, and your MCP URL becomes
`https://whatsapp.example.com` with the bearer token from
`/etc/whatsapp-mcp.env`. That is the URL Claude.ai and ChatGPT can use.

### Moving it later

Nothing ties it to one machine. The WhatsApp link lives in the data directory,
so copying it moves the session with no re-pairing and no new QR:

```bash
scp -r /var/lib/whatsapp-mcp   newbox:/var/lib/
scp /etc/whatsapp-mcp.env      newbox:/etc/
scp /usr/local/bin/whatsapp-mcp newbox:/usr/local/bin/
```

---

## 13. Build from source

```bash
git clone https://github.com/navidmoazzez/whatsapp-mcp.git
cd whatsapp-mcp
go build ./cmd/whatsapp-mcp
go test ./...
```

The binary is pure Go with no cgo, so it cross compiles anywhere:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/whatsapp-mcp
```

Layout:

| Path | What is in it |
|---|---|
| `cmd/whatsapp-mcp` | Entry point and flags |
| `internal/store` | SQLite schema, indexes, FTS5 search |
| `internal/wa` | WhatsApp link, pairing, history sync |
| `internal/mcpserver` | MCP tools |
| `internal/safety` | Send policy, allowlist, rate limit, audit log |

Issues and pull requests welcome.

---

## FAQ ❓

<details>
<summary><strong>What is an MCP server?</strong></summary>

Model Context Protocol is a standard way to give an AI assistant access to a
tool or a data source. An MCP server exposes a set of functions, and a client
like Claude Code or Claude Desktop calls them during a conversation. This one
exposes WhatsApp.

You install it once, point your client at it, and then ask in plain language.
You never call the tools yourself.
</details>

<details>
<summary><strong>Is this the official WhatsApp API?</strong></summary>

No. It uses the companion device protocol, the same one WhatsApp Web uses, so
it reaches your real personal inbox. WhatsApp does not support this for
automation.

The official Business API is a different thing entirely. It only reaches a
business number you own, cannot see a personal inbox, and outside a 24 hour
window it can only send pre-approved templates.
</details>

<details>
<summary><strong>Could my number get banned?</strong></summary>

Yes. WhatsApp bans accounts for automated behavior, and sending in volume is
the fastest way to get flagged. Reading is far lower risk than sending.

That is one reason sending is off by default and rate limited when you turn it
on.
</details>

<details>
<summary><strong>Do my messages get uploaded anywhere?</strong></summary>

Not by this software. There is no telemetry and no phone home. Your history
sits in a SQLite file on whatever machine you run it on.

Two exceptions you opt into. A hosted transcription provider receives the audio
of a voice note, and nothing else. And anything a tool returns enters your
conversation with the model, which goes wherever that model runs. That second
one is true of every MCP server and is the thing to understand before
installing this.
</details>

<details>
<summary><strong>Does my phone need to stay on?</strong></summary>

No. It links as a companion device, so once paired your phone can be asleep or
out of battery. The machine running the server needs to be awake, which is why
some people put it on a small always-on box.
</details>

<details>
<summary><strong>How far back does the history go?</strong></summary>

WhatsApp decides. It pushes a backlog when you pair, and how much varies. This
asks for as much as it will give, which in practice is months to years, but it
is never guaranteed complete.

Everything arriving from that moment on is captured in full.
</details>

<details>
<summary><strong>Can it read my voice notes?</strong></summary>

Yes, if you turn transcription on. They become searchable text alongside
everything you have typed, which is the single biggest blind spot in a WhatsApp
archive.

Four providers: local Whisper if nothing should leave your machine, or Groq,
OpenAI and ElevenLabs if you want it faster or more accurate across languages.
</details>

<details>
<summary><strong>Can it send as me?</strong></summary>

It can, and it is off by default. A companion device is your account, so
anything it sends is from you.

When you turn it on it previews first, obeys a chat allowlist, is rate limited,
and writes every attempt to a log no tool can edit.
</details>

<details>
<summary><strong>Can someone hide instructions in a message to hijack it?</strong></summary>

That is the real risk, and it is why sending is locked down. Every message in
your inbox is text somebody else wrote, and an agent that reads it and can also
send is exposed.

Nothing removes that risk entirely. [SECURITY.md](./SECURITY.md) covers what is
done about it and what is not.
</details>

<details>
<summary><strong>Does it work with groups?</strong></summary>

Yes. Group chats are searched and read like any other conversation, and sender
names are resolved so you can see who said what.
</details>

<details>
<summary><strong>What happens if I unlink it?</strong></summary>

It stops immediately. Log the device out under Linked Devices on your phone and
it loses access at once.

The history already downloaded stays on your machine until you delete the data
directory.
</details>

---

## About the author

Navid Moazzez is a leading AI business strategist, and the host of the AI Creator Summit, watched by 100,000+ creators. He helps creators and founders master AI and build their own AI Operating System (AI OS) to automate their business and life. This WhatsApp MCP server is one piece of that system.

## Dependencies

| Library | License | What it does |
|---|---|---|
| [whatsmeow](https://github.com/tulir/whatsmeow) | MPL-2.0 | The WhatsApp companion device protocol |
| [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) | Apache-2.0 | The MCP server and client |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | BSD-3-Clause | Pure Go SQLite, which is why there is no cgo |

## License

[MIT](./LICENSE). Free to use, modify, and share.

---

© 2026 NM Media. Made with ❤️ by [Navid Moazzez](https://navid.me).
