# WhatsApp channel for Claude Code

Optional. The MCP server works on its own, and most people will only want that.

This adds the other direction: message Claude from WhatsApp and have it answer
with your actual context, your repos, your skills, your connectors, your files.

Skip this entirely if you only want Claude to search and send WhatsApp when you
ask it to. Nothing in the main server depends on it.

This is the other half of [whatsapp-mcp](../). The MCP server is the filing
cabinet, holding your history and answering when asked. This is the doorbell,
telling Claude something arrived.

## How it fits together

```
your phone
    │
    ▼
WhatsApp
    │
    ▼
whatsapp-mcp on a server        always on, never misses a message
    │
    ▼
this channel, on your machine   pushes it into your live Claude session
    │
    ▼
Claude answers with your files loaded, and replies in the same chat
```

The split matters. The session has to stay connected to WhatsApp, which a
laptop that sleeps cannot do. But the useful answers need your machine, because
that is where your work is. So the server holds the line and your machine does
the thinking.

## Setup

1. Run a `whatsapp-mcp` server with `--http` and note its URL and token
2. Install this plugin and run `/whatsapp:configure`
3. Restart with `claude --channels plugin:whatsapp@whatsapp-mcp`

## Replying

Claude replies with the `reply` tool. Pass `speak: true` and the answer is sent
as a voice note in your own cloned voice rather than as text, which is worth
doing when you sent a voice note yourself.

## Security

Only chats on the allowlist reach Claude, and replies are refused to anything
else, so a message cannot persuade Claude to write to someone it should not.

Message text is data, not instructions. Everything arriving is written by other
people, and a channel that pushes it into a session with your files open is
exactly where prompt injection would aim.
