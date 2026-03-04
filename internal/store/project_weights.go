package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ProjectWeight is a local copy matching claude.ProjectWeight exactly for JSON compatibility.
// Defined here to avoid an import cycle (store cannot import claude).
type ProjectWeight struct {
	Project   string  `json:"project"`
	RepoRoot  string  `json:"repo_root"`
	Weight    float64 `json:"weight"`
	ToolCalls int     `json:"tool_calls"`
}

// SessionProjectWeightsStore reads and writes per-session multi-project weight data.
// Backed by a JSON file: {"session_id": [ProjectWeight, ...]}.
// Concurrent-safe for single-process use.
type SessionProjectWeightsStore struct {
	path string
	mu   sync.Mutex
}

// NewSessionProjectWeightsStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty map if absent.
func NewSessionProjectWeightsStore(path string) *SessionProjectWeightsStore {
	return &SessionProjectWeightsStore{path: path}
}

// Load reads all weights from disk.
// Returns an empty non-nil map if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *SessionProjectWeightsStore) Load() (map[string][]ProjectWeight, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]ProjectWeight{}, nil
		}
		return nil, err
	}

	var weights map[string][]ProjectWeight
	if err := json.Unmarshal(data, &weights); err != nil {
		return nil, err
	}
	return weights, nil
}

// Set writes or updates the project weights for sessionID.
// Creates the file and any parent directories if they do not exist.
// Reads current data, merges, and writes atomically (write-to-temp + rename).
func (s *SessionProjectWeightsStore) Set(sessionID string, weights []ProjectWeight) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.Load()
	if err != nil {
		return err
	}

	all[sessionID] = weights

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "project-weights-*.json")
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

// GetWeights returns the weights for a single session, or nil if not found.
func (s *SessionProjectWeightsStore) GetWeights(sessionID string) ([]ProjectWeight, error) {
	all, err := s.Load()
	if err != nil {
		return nil, err
	}
	weights, ok := all[sessionID]
	if !ok {
		return nil, nil
	}
	return weights, nil
}
