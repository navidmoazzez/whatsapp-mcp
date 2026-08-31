// Package agent lets an AI assistant answer WhatsApp messages by itself.
//
// A message arrives in one designated chat, a command is run with that message
// on stdin, and whatever it prints is sent back as a reply. That command is
// usually an AI CLI, so the chat becomes a conversation with an assistant.
//
// This is off unless explicitly configured, and it is worth understanding why
// before turning it on. The command runs on the machine hosting this server,
// so whoever can message the trigger chat can run it. Treat that chat as a
// terminal, because that is what it is.
package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Config configures the responder.
type Config struct {
	// ChatJID is the ONE chat that triggers a reply. Empty disables the whole
	// feature. Deliberately a single chat rather than a list: anyone able to
	// message a trigger chat can run the command, so the blast radius should
	// be something you chose on purpose.
	ChatJID string
	// Command and Args are what to run. The message text arrives on stdin.
	Command string
	Args    []string
	// Timeout bounds a single run. Zero uses two minutes.
	Timeout time.Duration
	// MaxReply caps the reply length. WhatsApp rejects very long messages, and
	// a runaway command should not try to send a novel. Zero uses 4000.
	MaxReply int
	// WorkDir is where the command runs.
	WorkDir string
}

// Responder runs the command and produces replies.
type Responder struct {
	cfg Config

	// One at a time. Two overlapping runs would interleave their replies and
	// double the load for no benefit, and a conversation is inherently serial.
	mu sync.Mutex
}

// New builds a Responder, or nil when the feature is off.
func New(cfg Config) (*Responder, error) {
	if strings.TrimSpace(cfg.ChatJID) == "" {
		return nil, nil
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("auto-reply needs a command to run. Pass --agent-command")
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return nil, fmt.Errorf("auto-reply command %q is not on the PATH: %w", cfg.Command, err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.MaxReply <= 0 {
		cfg.MaxReply = 4000
	}
	return &Responder{cfg: cfg}, nil
}

// Handles reports whether this chat is the trigger chat.
func (r *Responder) Handles(chatJID string) bool {
	return r != nil && chatJID == r.cfg.ChatJID
}

// Describe is reported by session_status.
func (r *Responder) Describe() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%s in %s", r.cfg.Command, r.cfg.ChatJID)
}

// Reply runs the command with the message on stdin and returns what to send.
//
// Errors come back as text rather than being swallowed. A silent failure in a
// chat looks identical to being ignored, which is the worst outcome: you sit
// there wondering whether it heard you.
func (r *Responder) Reply(ctx context.Context, message string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.cfg.Command, r.cfg.Args...)
	cmd.Dir = r.cfg.WorkDir
	cmd.Stdin = strings.NewReader(message)

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	reply := strings.TrimSpace(out.String())

	if ctx.Err() == context.DeadlineExceeded {
		// Keep whatever it managed to say. A long answer cut short is far more
		// useful than a bare complaint about the clock, and throwing the
		// output away loses work that was already done.
		if reply != "" {
			return truncate(reply, r.cfg.MaxReply) +
				fmt.Sprintf("\n\n[stopped after %s]", r.cfg.Timeout)
		}
		return fmt.Sprintf("That took longer than %s and was stopped.", r.cfg.Timeout)
	}
	if err != nil && reply == "" {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
		return "That failed: " + msg
	}
	if reply == "" {
		return "Done, with nothing to report."
	}

	return truncate(reply, r.cfg.MaxReply)
}

// truncate caps a reply. WhatsApp rejects very long messages, and a runaway
// command should not try to send a novel.
func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n\n[truncated]"
}
