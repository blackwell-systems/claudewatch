package ui

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

var (
	ErrNotTTY           = errors.New("not a TTY: cannot display interactive menu")
	ErrCancelled        = errors.New("selection cancelled by user")
	ErrInvalidSelection = errors.New("invalid selection")
)

// SelectSession displays an interactive numbered menu of sessions and prompts
// the user to select one.
// Returns the selected session's full ID, or an error.
func SelectSession(sessions []store.ActiveSession) (string, error) {
	// Check for empty list
	if len(sessions) == 0 {
		return "", errors.New("no sessions to select from")
	}

	// Check TTY
	if !IsTTY() {
		return "", ErrNotTTY
	}

	// Print header
	fmt.Fprintf(os.Stdout, "Multiple active sessions detected:\n\n")

	// Print numbered list
	for i, session := range sessions {
		sessionIDShort := session.SessionID
		if len(sessionIDShort) > 12 {
			sessionIDShort = sessionIDShort[:12]
		}
		timeAgo := formatTimeAgo(session.LastModified)
		fmt.Fprintf(os.Stdout, "  %d. %s  %-15s  (%s)\n",
			i+1, sessionIDShort, session.ProjectName, timeAgo)
	}

	// Print prompt
	fmt.Fprintf(os.Stdout, "\nSelect session (1-%d) or Ctrl+C to cancel: ", len(sessions))

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		// EOF or error - treat as cancelled
		return "", ErrCancelled
	}

	// Parse input
	response = strings.TrimSpace(response)
	selection, err := strconv.Atoi(response)
	if err != nil {
		return "", ErrInvalidSelection
	}

	// Validate range (1-indexed)
	if selection < 1 || selection > len(sessions) {
		return "", ErrInvalidSelection
	}

	// Return full session ID
	return sessions[selection-1].SessionID, nil
}

// formatTimeAgo formats a timestamp as a human-readable "X ago" string.
// Optimized for short durations (minutes/hours) as expected for active sessions.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}
