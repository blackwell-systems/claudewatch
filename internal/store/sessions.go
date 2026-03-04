package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ActiveSession represents a Claude Code session that has been recently active.
// This type is shared across multiple agents and consumers.
type ActiveSession struct {
	SessionID    string    // Full session ID (UUID)
	ProjectName  string    // Derived from ProjectPath (filepath.Base)
	LastModified time.Time // File modification time
	Path         string    // Full path to .jsonl file
}

// FindActiveSessions finds all .jsonl transcript files modified within the
// given duration threshold under claudeHome/projects/.
// Returns a list of active sessions sorted by last modified time (most recent first).
// Returns (nil, nil) if no active sessions are found.
func FindActiveSessions(claudeHome string, activeThreshold time.Duration) ([]ActiveSession, error) {
	projectsDir := filepath.Join(claudeHome, "projects")

	// Check if projects directory exists
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, nil
	}

	cutoffTime := time.Now().Add(-activeThreshold)
	var sessions []ActiveSession

	err := filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		// Get file info to check modification time
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr
		}

		// Only include files modified within the threshold
		if info.ModTime().Before(cutoffTime) {
			return nil
		}

		// Extract session ID from filename
		sessionID := strings.TrimSuffix(d.Name(), ".jsonl")

		// Extract project name from parent directory
		projectName := filepath.Base(filepath.Dir(path))

		sessions = append(sessions, ActiveSession{
			SessionID:    sessionID,
			ProjectName:  projectName,
			LastModified: info.ModTime(),
			Path:         path,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Return nil if no active sessions found
	if len(sessions) == 0 {
		return nil, nil
	}

	// Sort by last modified time, most recent first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	return sessions, nil
}
