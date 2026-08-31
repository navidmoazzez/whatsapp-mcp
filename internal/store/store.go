// Package store is the single source of truth for WhatsApp history.
//
// One process owns the database file and every caller goes through this
// package. Splitting reads and writes across two processes lets them disagree,
// which is a class of bug that simply cannot occur with a single owner.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, so the binary cross-compiles
)

// Store owns the message database.
type Store struct {
	db *sql.DB
}

// Message is a single row joined with the names we can resolve for it.
type Message struct {
	ID         string    `json:"id"`
	ChatJID    string    `json:"chat_jid"`
	ChatName   string    `json:"chat_name,omitempty"`
	SenderJID  string    `json:"sender_jid,omitempty"`
	SenderName string    `json:"sender_name,omitempty"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	IsFromMe   bool      `json:"is_from_me"`
	MsgType    string    `json:"type"`
	MediaType  string    `json:"media_type,omitempty"`
	Filename   string    `json:"filename,omitempty"`
	QuotedID   string    `json:"quoted_id,omitempty"`

	// Snippet is populated by Search only. It shows the matched terms in
	// context, so the model does not have to pull whole messages to see why
	// something matched.
	Snippet string `json:"snippet,omitempty"`
}

// Chat is a conversation, direct or group.
type Chat struct {
	JID      string    `json:"jid"`
	Name     string    `json:"name"`
	IsGroup  bool      `json:"is_group"`
	LastSeen time.Time `json:"last_message_time"`
}

// Contact is a person, resolved from push names and the address book.
type Contact struct {
	JID          string `json:"jid"`
	PushName     string `json:"push_name,omitempty"`
	BusinessName string `json:"business_name,omitempty"`
	FullName     string `json:"full_name,omitempty"`
}

// Open opens the database at path, creating it and its parent directory if
// needed, and applies the schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite takes a write lock per connection. More than one writer produces
	// SQLITE_BUSY under concurrent history sync, so serialize writes here.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// UpsertChat records a chat, keeping the newest last-message time.
func (s *Store) UpsertChat(ctx context.Context, c Chat) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats (jid, name, is_group, last_message_time)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			name              = CASE WHEN excluded.name <> '' THEN excluded.name ELSE chats.name END,
			is_group          = excluded.is_group,
			last_message_time = MAX(chats.last_message_time, excluded.last_message_time)`,
		c.JID, c.Name, c.IsGroup, c.LastSeen.Unix())
	return err
}

// UpsertContact records a contact. Empty fields never overwrite known ones,
// because push names arrive piecemeal and a later blank must not erase a name.
func (s *Store) UpsertContact(ctx context.Context, c Contact) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO contacts (jid, push_name, business_name, full_name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			push_name     = CASE WHEN excluded.push_name     <> '' THEN excluded.push_name     ELSE contacts.push_name     END,
			business_name = CASE WHEN excluded.business_name <> '' THEN excluded.business_name ELSE contacts.business_name END,
			full_name     = CASE WHEN excluded.full_name     <> '' THEN excluded.full_name     ELSE contacts.full_name     END`,
		c.JID, c.PushName, c.BusinessName, c.FullName)
	return err
}

// StoredMessage is what the WhatsApp layer hands in for persistence.
type StoredMessage struct {
	Message
	MediaURL   string
	MediaKey   []byte
	FileSHA256 []byte
	FileEncSHA []byte
	FileLength uint64
	MediaPath  string
	Transcript string
}

// UpsertMessage records a message. Re-delivery of the same ID updates rather
// than duplicating, which matters because history sync and live events overlap.
func (s *Store) UpsertMessage(ctx context.Context, m StoredMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (
			id, chat_jid, sender_jid, content, timestamp, is_from_me, msg_type,
			quoted_id, media_type, filename, media_url, media_key, file_sha256,
			file_enc_sha256, file_length, media_path, transcript
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(chat_jid, id) DO UPDATE SET
			content    = CASE WHEN excluded.content    <> '' THEN excluded.content    ELSE messages.content    END,
			media_path = CASE WHEN excluded.media_path <> '' THEN excluded.media_path ELSE messages.media_path END,
			transcript = CASE WHEN excluded.transcript <> '' THEN excluded.transcript ELSE messages.transcript END`,
		m.ID, m.ChatJID, m.SenderJID, m.Content, m.Timestamp.Unix(), m.IsFromMe,
		m.MsgType, m.QuotedID, m.MediaType, m.Filename, m.MediaURL, m.MediaKey,
		m.FileSHA256, m.FileEncSHA, m.FileLength, m.MediaPath, m.Transcript)
	return err
}

// selectMessage is the shared projection. sender_name prefers the address book
// name, then the business name, then the push name.
const selectMessage = `
	SELECT m.id, m.chat_jid,
	       COALESCE(NULLIF(c.name, ''), NULLIF(cc.full_name, ''), NULLIF(cc.push_name, ''), ''),
	       m.sender_jid,
	       COALESCE(NULLIF(ct.full_name, ''), NULLIF(ct.business_name, ''), NULLIF(ct.push_name, ''), ''),
	       m.content, m.timestamp, m.is_from_me, m.msg_type, m.media_type, m.filename, m.quoted_id
	FROM messages m
	LEFT JOIN chats    c  ON c.jid  = m.chat_jid
	LEFT JOIN contacts cc ON cc.jid = m.chat_jid
	LEFT JOIN contacts ct ON ct.jid = m.sender_jid`

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var ts int64
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.ChatName, &m.SenderJID,
			&m.SenderName, &m.Content, &ts, &m.IsFromMe, &m.MsgType,
			&m.MediaType, &m.Filename, &m.QuotedID); err != nil {
			return nil, err
		}
		m.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// MessageFilter narrows a message listing.
type MessageFilter struct {
	ChatJID string
	Since   *time.Time
	Until   *time.Time
	Limit   int
	Offset  int
}

// ListMessages returns messages newest first.
func (s *Store) ListMessages(ctx context.Context, f MessageFilter) ([]Message, error) {
	var where []string
	var args []any

	if f.ChatJID != "" {
		where = append(where, "m.chat_jid = ?")
		args = append(args, f.ChatJID)
	}
	if f.Since != nil {
		where = append(where, "m.timestamp >= ?")
		args = append(args, f.Since.Unix())
	}
	if f.Until != nil {
		where = append(where, "m.timestamp <= ?")
		args = append(args, f.Until.Unix())
	}
	where = append(where, "m.deleted = 0")

	q := selectMessage + " WHERE " + strings.Join(where, " AND ") +
		" ORDER BY m.timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, clampLimit(f.Limit), max(f.Offset, 0))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// Search runs a full-text query across message bodies and voice transcripts,
// ranked by bm25 rather than returned in arbitrary order.
//
// bm25 returns lower values for better matches, so ascending order puts the
// best match first.
func (s *Store) Search(ctx context.Context, query, chatJID string, limit int) ([]Message, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}

	q := `
		SELECT m.id, m.chat_jid,
		       COALESCE(NULLIF(c.name, ''), NULLIF(cc.full_name, ''), NULLIF(cc.business_name, ''), NULLIF(cc.push_name, ''), ''),
		       m.sender_jid,
		       COALESCE(NULLIF(ct.full_name, ''), NULLIF(ct.business_name, ''), NULLIF(ct.push_name, ''), ''),
		       m.content, m.timestamp, m.is_from_me, m.msg_type, m.media_type, m.filename, m.quoted_id,
		       snippet(messages_fts, 0, '[', ']', '...', 16)
		FROM messages_fts
		JOIN messages     m  ON m.rowid = messages_fts.rowid
		LEFT JOIN chats   c  ON c.jid   = m.chat_jid
		LEFT JOIN contacts cc ON cc.jid = m.chat_jid
		LEFT JOIN contacts ct ON ct.jid = m.sender_jid
		WHERE messages_fts MATCH ? AND m.deleted = 0`
	args := []any{match}

	if chatJID != "" {
		q += " AND m.chat_jid = ?"
		args = append(args, chatJID)
	}
	q += " ORDER BY bm25(messages_fts) LIMIT ?"
	args = append(args, clampLimit(limit))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var ts int64
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.ChatName, &m.SenderJID,
			&m.SenderName, &m.Content, &ts, &m.IsFromMe, &m.MsgType,
			&m.MediaType, &m.Filename, &m.QuotedID, &m.Snippet); err != nil {
			return nil, err
		}
		m.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListChats returns chats most recently active first. A non-empty query
// filters on name.
func (s *Store) ListChats(ctx context.Context, query string, limit, offset int) ([]Chat, error) {
	// A one to one chat carries no name of its own. WhatsApp never sends one,
	// because the name belongs to the person rather than the conversation, so
	// it has to come from the address book. Without this join a freshly synced
	// account shows hundreds of blank rows.
	q := `
		SELECT c.jid,
		       COALESCE(NULLIF(c.name, ''),
		                NULLIF(ct.full_name, ''),
		                NULLIF(ct.business_name, ''),
		                NULLIF(ct.push_name, ''),
		                -- Last resort: the phone number, so a chat is always
		                -- identifiable. Better an unlabelled number than a
		                -- blank row nobody can act on.
		                CASE WHEN c.jid LIKE '%@s.whatsapp.net'
		                     THEN '+' || substr(c.jid, 1, instr(c.jid, '@') - 1)
		                     -- A LID carries no phone number, so show the
		                     -- identifier. Unlabelled but addressable beats
		                     -- blank and invisible.
		                     ELSE 'Unknown (' || substr(c.jid, 1, instr(c.jid, '@') - 1) || ')'
		                END,
		                '') AS display_name,
		       c.is_group, c.last_message_time
		FROM chats c
		LEFT JOIN contacts ct ON ct.jid = c.jid`
	var args []any
	if query != "" {
		q += ` WHERE (c.name LIKE ? COLLATE NOCASE
		           OR ct.full_name LIKE ? COLLATE NOCASE
		           OR ct.push_name LIKE ? COLLATE NOCASE
		           OR c.jid LIKE ?)`
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY c.last_message_time DESC LIMIT ? OFFSET ?`
	args = append(args, clampLimit(limit), max(offset, 0))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		var ts int64
		if err := rows.Scan(&c.JID, &c.Name, &c.IsGroup, &ts); err != nil {
			return nil, err
		}
		c.LastSeen = time.Unix(ts, 0).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// SearchContacts looks in the contact table, not in chat names.
func (s *Store) SearchContacts(ctx context.Context, query string, limit int) ([]Contact, error) {
	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT jid, push_name, business_name, full_name
		FROM contacts
		WHERE (push_name LIKE ? COLLATE NOCASE
		    OR business_name LIKE ? COLLATE NOCASE
		    OR full_name LIKE ? COLLATE NOCASE
		    OR jid LIKE ?)
		  AND jid NOT LIKE '%@g.us'
		ORDER BY full_name, push_name
		LIMIT ?`, like, like, like, like, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.JID, &c.PushName, &c.BusinessName, &c.FullName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Counts reports how much history is stored, for the session health tool.
func (s *Store) Counts(ctx context.Context) (chats, messages int64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM chats), (SELECT COUNT(*) FROM messages)`).
		Scan(&chats, &messages)
	return
}

// ftsQuery turns free text into a safe FTS5 MATCH expression.
//
// FTS5 treats characters like ", *, ^, (, ) and NEAR as syntax, so raw user
// text is a syntax error waiting to happen. Every term is quoted, which makes
// it a literal, and the terms are ANDed.
func ftsQuery(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, `""`)
		quoted = append(quoted, `"`+f+`"`)
	}
	return strings.Join(quoted, " AND ")
}

func clampLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 500 {
		return 500
	}
	return n
}

// Media is everything needed to fetch and decrypt one attachment.
type Media struct {
	MediaType  string
	MediaURL   string
	MediaKey   []byte
	FileSHA256 []byte
	FileEncSHA []byte
	FileLength uint64
	Filename   string
	MediaPath  string
}

// GetMedia returns the attachment metadata stored with a message.
func (s *Store) GetMedia(ctx context.Context, chatJID, messageID string) (Media, error) {
	var m Media
	err := s.db.QueryRowContext(ctx, `
		SELECT media_type, media_url, media_key, file_sha256, file_enc_sha256,
		       file_length, filename, media_path
		FROM messages WHERE chat_jid = ? AND id = ?`, chatJID, messageID).
		Scan(&m.MediaType, &m.MediaURL, &m.MediaKey, &m.FileSHA256, &m.FileEncSHA,
			&m.FileLength, &m.Filename, &m.MediaPath)
	if err == sql.ErrNoRows {
		return m, fmt.Errorf("no message %s in chat %s", messageID, chatJID)
	}
	return m, err
}

// SetMediaPath records where a downloaded attachment landed, so a second
// request returns the existing file instead of downloading it again.
func (s *Store) SetMediaPath(ctx context.Context, chatJID, messageID, path string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE messages SET media_path = ? WHERE chat_jid = ? AND id = ?`,
		path, chatJID, messageID)
	return err
}

// FindDirectChat resolves a phone number or name to a one to one chat.
//
// A phone number is not a chat id, and a person may be keyed by a LID rather
// than their number, so this searches contacts as well as chats rather than
// assuming the caller can construct the JID.
func (s *Store) FindDirectChat(ctx context.Context, query string) ([]Chat, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, query)

	like := "%" + query + "%"
	numLike := "%" + digits + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.jid,
		       COALESCE(NULLIF(c.name,''), NULLIF(ct.full_name,''), NULLIF(ct.push_name,''), '') AS display_name,
		       c.is_group, c.last_message_time
		FROM chats c
		LEFT JOIN contacts ct ON ct.jid = c.jid
		WHERE c.is_group = 0
		  AND (   (? <> '' AND c.jid LIKE ?)
		       OR c.name LIKE ? COLLATE NOCASE
		       OR ct.full_name LIKE ? COLLATE NOCASE
		       OR ct.push_name LIKE ? COLLATE NOCASE)
		ORDER BY c.last_message_time DESC
		LIMIT 20`, digits, numLike, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		var ts int64
		if err := rows.Scan(&c.JID, &c.Name, &c.IsGroup, &ts); err != nil {
			return nil, err
		}
		c.LastSeen = time.Unix(ts, 0).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// MessageContext returns the messages surrounding one message.
//
// A search hit on its own is often unreadable: you get the line but not the
// exchange it belongs to. This is what turns a match into a conversation.
func (s *Store) MessageContext(ctx context.Context, chatJID, messageID string, before, after int) ([]Message, error) {
	if before <= 0 {
		before = 5
	}
	if after <= 0 {
		after = 5
	}

	var ts int64
	err := s.db.QueryRowContext(ctx,
		`SELECT timestamp FROM messages WHERE chat_jid = ? AND id = ?`, chatJID, messageID).Scan(&ts)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no message %s in chat %s", messageID, chatJID)
	}
	if err != nil {
		return nil, err
	}

	// Two windows either side, then merged, so the target sits in the middle
	// rather than at one end.
	q := selectMessage + `
		WHERE m.chat_jid = ? AND m.deleted = 0 AND m.timestamp <= ?
		ORDER BY m.timestamp DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, chatJID, ts, before+1)
	if err != nil {
		return nil, err
	}
	earlier, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}

	q2 := selectMessage + `
		WHERE m.chat_jid = ? AND m.deleted = 0 AND m.timestamp > ?
		ORDER BY m.timestamp ASC LIMIT ?`
	rows2, err := s.db.QueryContext(ctx, q2, chatJID, ts, after)
	if err != nil {
		return nil, err
	}
	later, err := scanMessages(rows2)
	if err != nil {
		return nil, err
	}

	// earlier is newest first and later is oldest first; return one run in
	// reading order.
	out := make([]Message, 0, len(earlier)+len(later))
	for i := len(earlier) - 1; i >= 0; i-- {
		out = append(out, earlier[i])
	}
	out = append(out, later...)
	return out, nil
}

// LastInteraction returns the most recent message exchanged with a chat.
func (s *Store) LastInteraction(ctx context.Context, chatJID string) (*Message, error) {
	msgs, err := s.ListMessages(ctx, MessageFilter{ChatJID: chatJID, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages stored for %s", chatJID)
	}
	return &msgs[0], nil
}
