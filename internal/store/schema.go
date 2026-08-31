package store

// schema is applied on every open. Every statement is idempotent.
//
// Three deliberate choices:
//
//  1. Timestamps are INTEGER unix seconds, not TIMESTAMP. They sort correctly,
//     they index, and they carry no timezone ambiguity.
//  2. Every column that is filtered or ordered on has an index. The reference
//     implementation has none, so it sorts the whole message table on every
//     list call.
//  3. Message bodies are mirrored into an FTS5 index. The reference does
//     LIKE '%term%' over lowercased content, which cannot use an index at all
//     and offers no phrase, boolean or ranked search.
//
// Foreign keys are deliberately not enforced. WhatsApp history sync delivers
// messages and chat metadata in no guaranteed order, and an enforced
// chat_jid reference silently drops messages that arrive before their chat.
const schema = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = OFF;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS chats (
	jid               TEXT PRIMARY KEY,
	name              TEXT    NOT NULL DEFAULT '',
	is_group          INTEGER NOT NULL DEFAULT 0,
	last_message_time INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_chats_recent ON chats(last_message_time DESC);

-- A real contact table. other implementations has none, so its
-- search_contacts is really a chat-name search and push names are never
-- resolved.
CREATE TABLE IF NOT EXISTS contacts (
	jid           TEXT PRIMARY KEY,
	push_name     TEXT NOT NULL DEFAULT '',
	business_name TEXT NOT NULL DEFAULT '',
	full_name     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_contacts_push ON contacts(push_name);
CREATE INDEX IF NOT EXISTS idx_contacts_full ON contacts(full_name);

CREATE TABLE IF NOT EXISTS messages (
	id              TEXT    NOT NULL,
	chat_jid        TEXT    NOT NULL,
	sender_jid      TEXT    NOT NULL DEFAULT '',
	content         TEXT    NOT NULL DEFAULT '',
	timestamp       INTEGER NOT NULL,
	is_from_me      INTEGER NOT NULL DEFAULT 0,
	msg_type        TEXT    NOT NULL DEFAULT 'text',
	quoted_id       TEXT    NOT NULL DEFAULT '',
	media_type      TEXT    NOT NULL DEFAULT '',
	filename        TEXT    NOT NULL DEFAULT '',
	media_url       TEXT    NOT NULL DEFAULT '',
	media_key       BLOB,
	file_sha256     BLOB,
	file_enc_sha256 BLOB,
	file_length     INTEGER NOT NULL DEFAULT 0,
	media_path      TEXT    NOT NULL DEFAULT '',
	transcript      TEXT    NOT NULL DEFAULT '',
	deleted         INTEGER NOT NULL DEFAULT 0,

	PRIMARY KEY (chat_jid, id)
);

-- chat_jid leads the primary key so one chat's messages sit together on disk.
CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_jid, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_time      ON messages(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender    ON messages(sender_jid, timestamp DESC);

-- External-content FTS5 index over message bodies and voice transcripts.
-- remove_diacritics 2 makes "Jose" match "José".
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	content,
	transcript,
	content      = 'messages',
	content_rowid = 'rowid',
	tokenize     = "unicode61 remove_diacritics 2"
);

CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, content, transcript)
	VALUES (new.rowid, new.content, new.transcript);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, content, transcript)
	VALUES ('delete', old.rowid, old.content, old.transcript);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, content, transcript)
	VALUES ('delete', old.rowid, old.content, old.transcript);
	INSERT INTO messages_fts(rowid, content, transcript)
	VALUES (new.rowid, new.content, new.transcript);
END;
`
