package store

import "time"

// ActiveSession represents a Claude Code session that has been recently active.
// This type is shared across multiple agents and consumers.
type ActiveSession struct {
	SessionID    string    // Full session ID (UUID)
	ProjectName  string    // Derived from ProjectPath (filepath.Base)
	LastModified time.Time // File modification time
	Path         string    // Full path to .jsonl file
}
