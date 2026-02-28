package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// ParseHistory reads ~/.claude/history.jsonl and returns all entries.
// It uses streaming JSONL parsing to handle large files efficiently.
func ParseHistory(claudeHome string) ([]HistoryEntry, error) {
	path := filepath.Join(claudeHome, "history.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	// Allow lines up to 1MB for large pasted contents.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry HistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines.
			continue
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// LatestSessionID returns the session ID of the most recent history entry.
func LatestSessionID(claudeHome string) (string, error) {
	entries, err := ParseHistory(claudeHome)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	latest := entries[0]
	for _, e := range entries[1:] {
		if e.Timestamp > latest.Timestamp {
			latest = e
		}
	}
	return latest.SessionID, nil
}
