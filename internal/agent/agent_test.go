package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOffWithoutAChat(t *testing.T) {
	r, err := New(Config{Command: "echo"})
	if err != nil {
		t.Fatalf("no chat should disable it quietly, got %v", err)
	}
	if r != nil {
		t.Error("no chat means the feature is off")
	}
	if r.Handles("anything@s.whatsapp.net") {
		t.Error("a nil responder must handle nothing")
	}
}

func TestRefusesAMissingCommand(t *testing.T) {
	if _, err := New(Config{ChatJID: "a@s.whatsapp.net"}); err == nil {
		t.Error("a chat with no command should be refused")
	}
	_, err := New(Config{ChatJID: "a@s.whatsapp.net", Command: "definitely-not-installed"})
	if err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("a missing binary should be caught at startup, got %v", err)
	}
}

// Exactly one chat triggers it. Anyone who can message that chat can run the
// command, so the blast radius must be something chosen on purpose.
func TestOnlyTheOneChatTriggers(t *testing.T) {
	r, err := New(Config{ChatJID: "trigger@s.whatsapp.net", Command: "echo"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !r.Handles("trigger@s.whatsapp.net") {
		t.Error("the trigger chat should be handled")
	}
	for _, other := range []string{"someone@s.whatsapp.net", "group@g.us", ""} {
		if r.Handles(other) {
			t.Errorf("%q must not trigger", other)
		}
	}
}

func TestReplyReturnsCommandOutput(t *testing.T) {
	r, _ := New(Config{ChatJID: "a@s.whatsapp.net", Command: "cat"})
	got := r.Reply(context.Background(), "hello from whatsapp")
	if got != "hello from whatsapp" {
		t.Errorf("want the command's output, got %q", got)
	}
}

// Silence in a chat is indistinguishable from being ignored, so a failure has
// to come back as words.
func TestFailureIsReportedNotSwallowed(t *testing.T) {
	r, _ := New(Config{ChatJID: "a@s.whatsapp.net", Command: "false"})
	got := r.Reply(context.Background(), "anything")
	if got == "" {
		t.Fatal("a failing command must still produce a reply")
	}
	if !strings.Contains(got, "failed") {
		t.Errorf("the reply should say it failed, got %q", got)
	}
}

func TestEmptyOutputStillReplies(t *testing.T) {
	r, _ := New(Config{ChatJID: "a@s.whatsapp.net", Command: "true"})
	if got := r.Reply(context.Background(), "x"); got == "" {
		t.Error("empty output must still send something, or it looks ignored")
	}
}

func TestSlowCommandIsStopped(t *testing.T) {
	r, _ := New(Config{
		ChatJID: "a@s.whatsapp.net", Command: "sleep", Args: []string{"30"},
		Timeout: 300 * time.Millisecond,
	})
	start := time.Now()
	got := r.Reply(context.Background(), "x")
	if time.Since(start) > 5*time.Second {
		t.Error("the timeout did not stop it")
	}
	if !strings.Contains(got, "longer") {
		t.Errorf("the reply should explain the timeout, got %q", got)
	}
}

func TestLongOutputIsTruncated(t *testing.T) {
	r, _ := New(Config{
		ChatJID: "a@s.whatsapp.net", Command: "yes", Args: []string{"padding"},
		Timeout: time.Second, MaxReply: 200,
	})
	got := r.Reply(context.Background(), "x")
	if len(got) > 260 {
		t.Errorf("want a truncated reply, got %d chars", len(got))
	}
	// Either marker is correct: it was cut for length, or cut by the clock
	// with its partial output kept.
	if !strings.Contains(got, "truncated") && !strings.Contains(got, "stopped after") {
		t.Errorf("the cut should be visible in the reply, got %q", got[max(0, len(got)-60):])
	}
}
