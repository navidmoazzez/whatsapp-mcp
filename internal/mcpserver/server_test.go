package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/navidmoazzez/whatsapp-mcp/internal/safety"
	"github.com/navidmoazzez/whatsapp-mcp/internal/store"
	"github.com/navidmoazzez/whatsapp-mcp/internal/wa"
)

type fakeWA struct {
	sent []string
}

func (f *fakeWA) Status() wa.Status {
	return wa.Status{Paired: true, Connected: true, JID: "1@s.whatsapp.net"}
}

func (f *fakeWA) SendText(_ context.Context, chatJID, text string) (string, error) {
	f.sent = append(f.sent, chatJID+": "+text)
	return "MSGID1", nil
}

func (f *fakeWA) DownloadMedia(_ context.Context, chatJID, messageID string) (string, error) {
	return "/tmp/" + messageID + ".jpg", nil
}

func (f *fakeWA) SendFile(_ context.Context, chatJID, path, caption string) (string, error) {
	f.sent = append(f.sent, chatJID+": file "+path)
	return "MSGID2", nil
}

func (f *fakeWA) SendVoiceBytes(_ context.Context, chatJID string, ogg []byte) (string, error) {
	f.sent = append(f.sent, chatJID+": voicebytes")
	return "MSGID4", nil
}

func (f *fakeWA) SendAudioBytes(_ context.Context, chatJID string, data []byte, filename string) (string, error) {
	f.sent = append(f.sent, chatJID+": audiobytes")
	return "MSGID5", nil
}

func (f *fakeWA) SendVoiceNote(_ context.Context, chatJID, path string) (string, error) {
	f.sent = append(f.sent, chatJID+": voice "+path)
	return "MSGID3", nil
}

// session wires a real MCP client to a real MCP server over the SDK's
// in-memory transport, so these tests exercise the actual protocol.
func session(t *testing.T, allowSend bool, allowlist ...string) (*mcp.ClientSession, *fakeWA, *store.Store) {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	guard, err := safety.New(safety.Config{
		AllowSend: allowSend,
		Allowlist: allowlist,
		AuditPath: filepath.Join(t.TempDir(), "audit.log"),
	})
	if err != nil {
		t.Fatalf("guard: %v", err)
	}

	fake := &fakeWA{}
	srv := New(Deps{Store: st, Client: fake, Guard: guard})

	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).
		Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return cs, fake, st
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: name, Arguments: args,
	})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func TestToolsAreAdvertisedWithAnnotations(t *testing.T) {
	cs, _, _ := session(t, false)

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}

	for _, want := range []string{
		"search_messages", "list_messages", "list_chats",
		"search_contacts", "session_status", "send_message",
		"download_media", "send_file", "send_voice_note",
	} {
		if byName[want] == nil {
			t.Errorf("tool %s is not advertised", want)
		}
	}

	// A client must be able to tell a read from a write without calling it.
	if tool := byName["search_messages"]; tool != nil {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Error("search_messages should be annotated read-only")
		}
		if tool.OutputSchema == nil {
			t.Error("search_messages should declare an output schema")
		}
	}
	if tool := byName["send_message"]; tool != nil {
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
			t.Error("send_message must not be annotated read-only")
		}
		if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
			t.Error("send_message should be annotated destructive")
		}
	}
}

func TestSearchReturnsStructuredContent(t *testing.T) {
	cs, _, st := session(t, false)

	err := st.UpsertMessage(context.Background(), store.StoredMessage{Message: store.Message{
		ID: "1", ChatJID: "a@s.whatsapp.net", Content: "the invoice is overdue",
		Timestamp: time.Now(), MsgType: "text",
	}})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	res := call(t, cs, "search_messages", map[string]any{"query": "invoice"})
	if res.IsError {
		t.Fatalf("search returned an error result")
	}
	if res.StructuredContent == nil {
		t.Fatal("want structured content, got none")
	}

	raw, _ := json.Marshal(res.StructuredContent)
	var out messagesOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count != 1 || len(out.Messages) != 1 {
		t.Fatalf("want 1 hit, got %d", out.Count)
	}
	if out.Messages[0].Snippet == "" {
		t.Error("want a highlighted snippet")
	}
	if out.Note == "" {
		t.Error("want the untrusted-content note attached to other people's words")
	}
}

// The headline safety property: a default install cannot send.
func TestSendIsRefusedByDefault(t *testing.T) {
	cs, fake, _ := session(t, false)

	res := call(t, cs, "send_message", map[string]any{
		"chat_jid": "a@s.whatsapp.net", "text": "hello", "confirm": true,
	})
	if !res.IsError {
		t.Fatal("send should be refused when --allow-send was not passed")
	}
	if len(fake.sent) != 0 {
		t.Fatalf("nothing should have been sent, got %v", fake.sent)
	}
}

// Even with sending enabled, the first call is a preview.
func TestSendRequiresConfirmation(t *testing.T) {
	cs, fake, _ := session(t, true)

	res := call(t, cs, "send_message", map[string]any{
		"chat_jid": "a@s.whatsapp.net", "text": "hello",
	})
	if res.IsError {
		t.Fatal("an unconfirmed send should preview, not error")
	}
	if len(fake.sent) != 0 {
		t.Fatalf("preview must not send, got %v", fake.sent)
	}

	res = call(t, cs, "send_message", map[string]any{
		"chat_jid": "a@s.whatsapp.net", "text": "hello", "confirm": true,
	})
	if res.IsError {
		t.Fatal("a confirmed send should succeed")
	}
	if len(fake.sent) != 1 {
		t.Fatalf("want exactly 1 send, got %v", fake.sent)
	}
}

func TestSendRespectsAllowlist(t *testing.T) {
	cs, fake, _ := session(t, true, "friend@s.whatsapp.net")

	res := call(t, cs, "send_message", map[string]any{
		"chat_jid": "stranger@s.whatsapp.net", "text": "hi", "confirm": true,
	})
	if !res.IsError {
		t.Fatal("a chat outside the allowlist must be refused")
	}

	res = call(t, cs, "send_message", map[string]any{
		"chat_jid": "friend@s.whatsapp.net", "text": "hi", "confirm": true,
	})
	if res.IsError {
		t.Fatal("a chat on the allowlist should be permitted")
	}
	if len(fake.sent) != 1 {
		t.Fatalf("want 1 send, got %v", fake.sent)
	}
}

func TestSessionStatusExplainsItself(t *testing.T) {
	cs, _, _ := session(t, false)

	res := call(t, cs, "session_status", map[string]any{})
	raw, _ := json.Marshal(res.StructuredContent)
	var out statusOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.ReadOnly {
		t.Error("want read_only true on a default install")
	}
	if out.Explained == "" {
		t.Error("want a plain-language explanation of the current state")
	}
}

func TestBadTimestampGivesAUsefulError(t *testing.T) {
	cs, _, _ := session(t, false)

	res := call(t, cs, "list_messages", map[string]any{"since": "yesterday"})
	if !res.IsError {
		t.Fatal("an unparseable timestamp should be an error")
	}
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}
	if !strings.Contains(text, "RFC3339") {
		t.Errorf("the error should say what format is expected, got %q", text)
	}
}

// Ogg Opus must be detected from the header, not the filename. A recording
// renamed to .ogg is still not Opus, and sending it as a voice note makes
// WhatsApp draw a waveform for audio it cannot decode, so it arrives silent.
func TestOggOpusDetectedFromHeaderNotName(t *testing.T) {
	oggOpus := append([]byte("OggS"), make([]byte, 24)...)
	oggOpus = append(oggOpus, []byte("OpusHead")...)
	oggOpus = append(oggOpus, make([]byte, 16)...)

	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"real ogg opus", oggOpus, true},
		{"mp3 bytes", append([]byte("ID3\x04"), make([]byte, 60)...), false},
		{"mp4 bytes", append([]byte("\x00\x00\x00\x20ftypM4A "), make([]byte, 60)...), false},
		{"ogg vorbis, not opus", append([]byte("OggS"), make([]byte, 60)...), false},
		{"empty", nil, false},
		{"too short to judge", []byte("OggS"), false},
	}

	for _, c := range cases {
		if got := isOggOpus(c.in); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestSendAudioRefusedByDefault(t *testing.T) {
	cs, fake, _ := session(t, false)

	res := call(t, cs, "send_audio", map[string]any{
		"chat_jid": "a@s.whatsapp.net", "audio_base64": "T2dnUw==", "confirm": true,
	})
	if !res.IsError {
		t.Fatal("send_audio must obey the same guard as send_message")
	}
	if len(fake.sent) != 0 {
		t.Fatalf("nothing should have been sent, got %v", fake.sent)
	}
}

func TestSendAudioRejectsBadBase64(t *testing.T) {
	cs, _, _ := session(t, true)

	res := call(t, cs, "send_audio", map[string]any{
		"chat_jid": "a@s.whatsapp.net", "audio_base64": "not base64 at all !!!", "confirm": true,
	})
	if !res.IsError {
		t.Fatal("invalid base64 should be an error, not a silent failure")
	}
}
