package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/blackwell-systems/claudewatch/internal/ui"
)

// SessionSelectOptions configures session selection behavior.
type SessionSelectOptions struct {
	allowHistoricalFallback bool
	requireActive           bool
}

// SessionSelectOption is a functional option for configuring session selection.
type SessionSelectOption func(*SessionSelectOptions)

// WithMostRecentFallback allows falling back to the most recent session from
// all sessions if no active sessions are found. This is the default behavior.
func WithMostRecentFallback() SessionSelectOption {
	return func(o *SessionSelectOptions) {
		o.allowHistoricalFallback = true
	}
}

// RequireActiveSession requires that a session must be active (modified within
// the threshold). If no active sessions exist, returns an error instead of
// falling back to historical sessions.
func RequireActiveSession() SessionSelectOption {
	return func(o *SessionSelectOptions) {
		o.requireActive = true
		o.allowHistoricalFallback = false
	}
}

// SelectSession returns a session ID using the following logic:
//  1. If flagValue is non-empty, return it directly (explicit flag wins)
//  2. Find active sessions (modified within 15 minutes)
//  3. If multiple active sessions exist:
//     - TTY: show interactive menu
//     - Non-TTY: return error with session list
//  4. If single active session: return it
//  5. If no active sessions:
//     - RequireActiveSession: return error
//     - WithMostRecentFallback (default): return most recent session from all sessions
func SelectSession(cfg *config.Config, flagValue string, opts ...SessionSelectOption) (string, error) {
	// Apply options
	options := &SessionSelectOptions{
		allowHistoricalFallback: true, // default
	}
	for _, opt := range opts {
		opt(options)
	}

	// Explicit flag value wins
	if flagValue != "" {
		return flagValue, nil
	}

	// Find active sessions
	activeSessions, err := store.FindActiveSessions(cfg.ClaudeHome, 15*time.Minute)
	if err != nil {
		return "", fmt.Errorf("finding active sessions: %w", err)
	}

	switch len(activeSessions) {
	case 0:
		// No active sessions
		if options.requireActive {
			return "", fmt.Errorf("no active sessions found (use --session to specify a session ID)")
		}
		if options.allowHistoricalFallback {
			return findMostRecentSession(cfg.ClaudeHome)
		}
		return "", fmt.Errorf("no active sessions found")

	case 1:
		// Single active session - use it
		return activeSessions[0].SessionID, nil

	default:
		// Multiple active sessions
		if !ui.IsTTY() {
			return "", formatMultipleSessionsError(activeSessions)
		}
		return ui.SelectSession(activeSessions)
	}
}

// findMostRecentSession returns the most recent session ID from all sessions.
func findMostRecentSession(claudeHome string) (string, error) {
	sessions, err := claude.ParseAllSessionMeta(claudeHome)
	if err != nil {
		return "", fmt.Errorf("parsing sessions: %w", err)
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found")
	}

	// Sort by start time descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime > sessions[j].StartTime
	})

	return sessions[0].SessionID, nil
}

// formatMultipleSessionsError returns a formatted error message with the list
// of active sessions, for non-TTY environments.
func formatMultipleSessionsError(sessions []store.ActiveSession) error {
	var b strings.Builder
	b.WriteString("multiple active sessions found (use --session to specify):\n")

	for i, s := range sessions {
		sessionIDShort := s.SessionID
		if len(sessionIDShort) > 12 {
			sessionIDShort = sessionIDShort[:12]
		}

		timeAgo := formatTimeAgo(s.LastModified)
		fmt.Fprintf(&b, "  %d. %s  %-20s  (%s)\n",
			i+1, sessionIDShort, s.ProjectName, timeAgo)
	}

	return errors.New(b.String())
}

// formatTimeAgo formats a time as "2m ago", "14m ago", "2h ago", etc.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
