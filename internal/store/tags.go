package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SessionTagStore reads and writes session project name overrides.
// Backed by a JSON file containing a flat map: {"session_id": "project_name"}.
// Concurrent-safe for single-process use (Set holds a mutex).
type SessionTagStore struct {
	path string
	mu   sync.Mutex
}

// NewSessionTagStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty map if absent.
func NewSessionTagStore(path string) *SessionTagStore {
	return &SessionTagStore{path: path}
}

// Load reads all tags from disk.
// Returns an empty non-nil map if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *SessionTagStore) Load() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	var tags map[string]string
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// Set writes or updates the project name override for sessionID.
// Creates the file and any parent directories if they do not exist.
// Reads current tags, merges, and writes atomically (write-to-temp + rename).
func (s *SessionTagStore) Set(sessionID, projectName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tags, err := s.Load()
	if err != nil {
		return err
	}

	tags[sessionID] = projectName

	data, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "tags-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	return nil
}
