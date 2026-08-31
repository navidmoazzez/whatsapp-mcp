---
name: configure
description: Point the WhatsApp channel at your whatsapp-mcp server so messages reach this session. Use when setting up the WhatsApp channel or when it reports it is not configured.
---

# Configure the WhatsApp channel

Writes `~/.claude/channels/whatsapp/.env`, outside the plugin directory so a
plugin update never overwrites it.

Three values are needed:

| Key | What it is |
|---|---|
| `WHATSAPP_MCP_URL` | Your whatsapp-mcp server, for example `https://whatsapp.example.com/` |
| `WHATSAPP_MCP_TOKEN` | Its bearer token, from `/etc/whatsapp-mcp.env` on that server |
| `WHATSAPP_ALLOW_CHATS` | Comma separated chat ids allowed to reach Claude |

Write the file with mode 600, since it holds a token that reaches an entire
WhatsApp history.

```bash
mkdir -p ~/.claude/channels/whatsapp
cat > ~/.claude/channels/whatsapp/.env <<'ENV'
WHATSAPP_MCP_URL=https://whatsapp.example.com/
WHATSAPP_MCP_TOKEN=wamcp_...
WHATSAPP_ALLOW_CHATS=1234567890@s.whatsapp.net
ENV
chmod 600 ~/.claude/channels/whatsapp/.env
```

To find the chat id, ask the whatsapp-mcp server for `list_chats` and take the
`jid` of the conversation you want to use.

Then restart Claude Code with the channel enabled:

```bash
claude --channels plugin:whatsapp@whatsapp-mcp
```

While the channel is a custom one rather than an Anthropic-approved plugin, it
needs `--dangerously-load-development-channels` instead.

## The allowlist is the security boundary

Anyone whose chat is listed can push text straight into a Claude session that
has your files open. Nothing else reaches Claude, and replies are refused to
any chat not on the list, so a message cannot talk Claude into writing to
somebody else.

Add only chats you control.
