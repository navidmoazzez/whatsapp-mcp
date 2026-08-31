package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seed(t *testing.T, s *Store, msgs ...StoredMessage) {
	t.Helper()
	ctx := context.Background()
	for _, m := range msgs {
		if err := s.UpsertMessage(ctx, m); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
}

func msg(id, chat, content string, ts time.Time) StoredMessage {
	return StoredMessage{Message: Message{
		ID: id, ChatJID: chat, Content: content, Timestamp: ts, MsgType: "text",
	}}
}

// The whole design rests on FTS5 being present in the pure Go driver. If it is
// not, Open fails here rather than in a user's terminal.
func TestSchemaAppliesWithFTS5(t *testing.T) {
	newTestStore(t)
}

func TestSearchRanksAndSnippets(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	seed(t, s,
		msg("1", "a@s.whatsapp.net", "let us settle the pricing for the retainer", now),
		msg("2", "a@s.whatsapp.net", "lunch on thursday sounds good", now.Add(time.Minute)),
		msg("3", "b@s.whatsapp.net", "pricing pricing pricing is the whole problem", now.Add(2*time.Minute)),
	)

	got, err := s.Search(ctx, "pricing", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 hits, got %d", len(got))
	}
	// bm25 must put the denser match first. Recency ordering would not.
	if got[0].ID != "3" {
		t.Errorf("want best match id 3 first, got %s", got[0].ID)
	}
	if got[0].Snippet == "" {
		t.Error("want a snippet, got none")
	}
}

// Diacritic folding is why remove_diacritics 2 is set on the tokenizer.
func TestSearchFoldsDiacritics(t *testing.T) {
	s := newTestStore(t)
	seed(t, s, msg("1", "a@s.whatsapp.net", "dinner with José on friday", time.Now()))

	got, err := s.Search(context.Background(), "jose", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 hit for unaccented query, got %d", len(got))
	}
}

// Raw user text reaches MATCH, so FTS5 syntax characters must not blow up.
func TestSearchSurvivesFTSSyntaxInput(t *testing.T) {
	s := newTestStore(t)
	seed(t, s, msg("1", "a@s.whatsapp.net", "the quote was \"fifty\" per seat", time.Now()))

	for _, q := range []string{`"`, `NEAR(`, `*`, `^foo`, `a OR b)`, `""`} {
		if _, err := s.Search(context.Background(), q, "", 10); err != nil {
			t.Errorf("query %q should not error, got %v", q, err)
		}
	}
}

// Live events and history sync deliver the same message twice.
func TestUpsertMessageIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	seed(t, s,
		msg("1", "a@s.whatsapp.net", "hello", now),
		msg("1", "a@s.whatsapp.net", "hello", now),
	)

	_, count, err := s.Counts(ctx)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1 stored message after double delivery, got %d", count)
	}

	// A redelivery with empty content must not erase the body.
	seed(t, s, msg("1", "a@s.whatsapp.net", "", now))
	got, err := s.ListMessages(ctx, MessageFilter{ChatJID: "a@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Content != "hello" {
		t.Errorf("blank redelivery erased content: %+v", got)
	}
}

// Messages must survive arriving before their chat row. An enforced foreign
// key is what silently drops them in the implementation this replaces.
func TestMessageBeforeChatIsKept(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed(t, s, msg("1", "orphan@s.whatsapp.net", "arrived first", time.Now()))
	if err := s.UpsertChat(ctx, Chat{JID: "orphan@s.whatsapp.net", Name: "Later"}); err != nil {
		t.Fatalf("upsert chat: %v", err)
	}

	got, err := s.ListMessages(ctx, MessageFilter{ChatJID: "orphan@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want the orphan message kept, got %d", len(got))
	}
	if got[0].ChatName != "Later" {
		t.Errorf("want chat name resolved after the fact, got %q", got[0].ChatName)
	}
}

// Editing content must reindex, or search returns stale hits.
func TestFTSStaysInSyncOnUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	seed(t, s, msg("1", "a@s.whatsapp.net", "original wording", now))
	seed(t, s, msg("1", "a@s.whatsapp.net", "replacement wording", now))

	if got, _ := s.Search(ctx, "replacement", "", 10); len(got) != 1 {
		t.Errorf("want the updated text searchable, got %d hits", len(got))
	}
	if got, _ := s.Search(ctx, "original", "", 10); len(got) != 0 {
		t.Errorf("want the old text gone from the index, got %d hits", len(got))
	}
}

// A one to one chat has no name of its own, so it must come from contacts.
// Without this, a freshly synced account shows hundreds of blank chats.
func TestChatNameFallsBackToContact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, Chat{JID: "46701234567@s.whatsapp.net", LastSeen: time.Now()}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := s.UpsertContact(ctx, Contact{JID: "46701234567@s.whatsapp.net", PushName: "Sara"}); err != nil {
		t.Fatalf("contact: %v", err)
	}

	got, err := s.ListChats(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Sara" {
		t.Fatalf("want the contact name used for an unnamed chat, got %+v", got)
	}
}

// A group has its own name, which must win over any contact row.
func TestGroupNameWinsOverContact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, Chat{JID: "123@g.us", Name: "Founders", IsGroup: true, LastSeen: time.Now()}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := s.UpsertContact(ctx, Contact{JID: "123@g.us", PushName: "Wrong"}); err != nil {
		t.Fatalf("contact: %v", err)
	}

	got, _ := s.ListChats(ctx, "", 10, 0)
	if len(got) != 1 || got[0].Name != "Founders" {
		t.Fatalf("want the group's own name, got %+v", got)
	}
}

// Searching chats must find a person by their contact name, not only by a
// chat name that does not exist for one to one conversations.
func TestSearchChatsByContactName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.UpsertChat(ctx, Chat{JID: "46701234567@s.whatsapp.net", LastSeen: time.Now()})
	_ = s.UpsertContact(ctx, Contact{JID: "46701234567@s.whatsapp.net", FullName: "Sara Lindqvist"})

	got, err := s.ListChats(ctx, "lindqvist", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want to find the chat by contact name, got %d", len(got))
	}
}

// A message's chat_name must resolve the same way as the chat list, or a
// search result shows a phone number where the list shows a name.
func TestMessageChatNameFallsBackToContact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.UpsertChat(ctx, Chat{JID: "46701234567@s.whatsapp.net", LastSeen: time.Now()})
	_ = s.UpsertContact(ctx, Contact{JID: "46701234567@s.whatsapp.net", PushName: "Sara"})
	seed(t, s, msg("1", "46701234567@s.whatsapp.net", "hello there", time.Now()))

	got, err := s.ListMessages(ctx, MessageFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ChatName != "Sara" {
		t.Fatalf("want the contact name on the message, got %+v", got)
	}
}

// A chat must never be renamed after the account owner. Writing the sender's
// push name unconditionally renamed every thread after you the moment you
// replied to it.
func TestOwnNameNeverOverwritesAChatName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	jid := "555000111222333@lid"

	// An inbound message names the chat after the other person.
	if err := s.UpsertChat(ctx, Chat{JID: jid, Name: "Wren", LastSeen: time.Now()}); err != nil {
		t.Fatalf("inbound: %v", err)
	}

	// A reply must not rename it. The write path passes an empty name for
	// outbound messages, and an empty name must never overwrite a stored one.
	if err := s.UpsertChat(ctx, Chat{JID: jid, Name: "", LastSeen: time.Now().Add(time.Minute)}); err != nil {
		t.Fatalf("outbound: %v", err)
	}

	got, err := s.ListChats(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Wren" {
		t.Fatalf("a reply renamed the chat: %+v", got)
	}
}
