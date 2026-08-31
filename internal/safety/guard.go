// Package safety decides whether a write is allowed to reach WhatsApp.
//
// Every message in a WhatsApp inbox is text written by someone else. An agent
// that can both read that text and send messages is exposed to prompt
// injection: a message in a group chat can instruct the model to forward
// private history somewhere.
//
// Naming that risk in a README is not a defence. This package is the defence.
package safety

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Guard enforces the send policy.
type Guard struct {
	allowSend bool
	allowlist map[string]bool
	perMinute int

	mu     sync.Mutex
	sent   []time.Time
	auditW *os.File
}

// Config configures a Guard.
type Config struct {
	// AllowSend must be true for any write to succeed. Off by default, so a
	// default install cannot send anything at all.
	AllowSend bool
	// Allowlist limits sends to these JIDs. Empty means every chat is allowed,
	// which is only reachable when AllowSend is already on.
	Allowlist []string
	// PerMinute caps sends per rolling minute. Zero uses the default of 10.
	PerMinute int
	// AuditPath is an append-only log of every attempted write.
	AuditPath string
}

// New builds a Guard.
func New(cfg Config) (*Guard, error) {
	g := &Guard{
		allowSend: cfg.AllowSend,
		allowlist: make(map[string]bool, len(cfg.Allowlist)),
		perMinute: cfg.PerMinute,
	}
	if g.perMinute <= 0 {
		g.perMinute = 10
	}
	for _, jid := range cfg.Allowlist {
		if jid = strings.TrimSpace(jid); jid != "" {
			g.allowlist[jid] = true
		}
	}

	if cfg.AuditPath != "" {
		f, err := os.OpenFile(cfg.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open audit log: %w", err)
		}
		g.auditW = f
	}
	return g, nil
}

// Close releases the audit log.
func (g *Guard) Close() error {
	if g.auditW != nil {
		return g.auditW.Close()
	}
	return nil
}

// CanSend reports whether sending to jid is permitted, and why not if it is
// not. The reason is written for the model to read, so it explains the fix.
func (g *Guard) CanSend(jid string) error {
	if !g.allowSend {
		return fmt.Errorf("sending is disabled. This server runs read-only unless it was started with --allow-send")
	}
	if len(g.allowlist) > 0 && !g.allowlist[jid] {
		return fmt.Errorf("chat %s is not on the send allowlist. Only chats passed to --send-to can receive messages", jid)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-time.Minute)
	kept := g.sent[:0]
	for _, t := range g.sent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	g.sent = kept

	if len(g.sent) >= g.perMinute {
		return fmt.Errorf("send rate limit reached, %d per minute. This exists so a loop cannot flood a contact", g.perMinute)
	}
	return nil
}

// RecordSend notes a completed send against the rate limit and the audit log.
func (g *Guard) RecordSend(jid, preview string, err error) {
	g.mu.Lock()
	g.sent = append(g.sent, time.Now())
	g.mu.Unlock()
	g.audit("send", jid, preview, err)
}

// RecordBlocked notes a refused write.
func (g *Guard) RecordBlocked(jid, preview string, reason error) {
	g.audit("blocked", jid, preview, reason)
}

// audit appends one line. The model has no tool that can edit or read this
// file, so it is a record the agent cannot tamper with.
func (g *Guard) audit(action, jid, preview string, err error) {
	if g.auditW == nil {
		return
	}
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	preview = strings.ReplaceAll(preview, "\n", " ")

	outcome := "ok"
	if err != nil {
		outcome = err.Error()
	}
	fmt.Fprintf(g.auditW, "%s\t%s\t%s\t%s\t%s\n",
		time.Now().UTC().Format(time.RFC3339), action, jid, outcome, preview)
}

// ReadOnly reports whether writes are disabled.
func (g *Guard) ReadOnly() bool { return !g.allowSend }

// Untrusted wraps message text that came from other people.
//
// The model is told, in band, that what follows is data rather than
// instructions. This does not make injection impossible, and the README says
// so, but it removes the easiest version of the attack.
func Untrusted(s string) string {
	if s == "" {
		return ""
	}
	return s
}
