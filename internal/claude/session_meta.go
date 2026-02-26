package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ParseAllSessionMeta reads all JSON files from ~/.claude/usage-data/session-meta/
// and returns parsed SessionMeta entries.
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error) {
	dir := filepath.Join(claudeHome, "usage-data", "session-meta")
	return parseJSONDir[SessionMeta](dir)
}

// ParseSessionMeta reads a single session meta file.
func ParseSessionMeta(path string) (*SessionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// parseJSONDir reads all .json files from a directory and unmarshals them
// into a slice of the given type. Skips files that fail to parse.
func parseJSONDir[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []T
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}
