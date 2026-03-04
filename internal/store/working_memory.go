package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TaskMemory represents a single task's history across sessions.
type TaskMemory struct {
	TaskIdentifier string    `json:"task_identifier"`
	Sessions       []string  `json:"sessions"`     // session IDs that worked on this task
	Status         string    `json:"status"`       // "completed", "abandoned", "in_progress"
	BlockersHit    []string  `json:"blockers_hit"` // descriptions of blockers encountered
	Solution       string    `json:"solution"`     // how it was resolved (empty if abandoned)
	Commits        []string  `json:"commits"`      // commit SHAs produced
	LastUpdated    time.Time `json:"last_updated"`
}

// BlockerMemory represents a known blocker for this project.
type BlockerMemory struct {
	File        string    `json:"file"`        // file path (if file-specific)
	Issue       string    `json:"issue"`       // description of the problem
	Solution    string    `json:"solution"`    // how to resolve (if known)
	Encountered []string  `json:"encountered"` // dates encountered (YYYY-MM-DD format)
	LastSeen    time.Time `json:"last_seen"`
}

// WorkingMemory is the root structure stored in working-memory.json.
type WorkingMemory struct {
	Tasks        map[string]*TaskMemory `json:"tasks"` // keyed by task_identifier
	Blockers     []*BlockerMemory       `json:"blockers"`
	ContextHints []string               `json:"context_hints"` // frequently needed files
	LastScanned  time.Time              `json:"last_scanned"`  // last time memory was updated
}

// WorkingMemoryStore reads and writes working memory data.
// Backed by a JSON file storing WorkingMemory structure.
// Concurrent-safe for single-process use.
type WorkingMemoryStore struct {
	path string
	mu   sync.Mutex
}

// NewWorkingMemoryStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty WorkingMemory if absent.
func NewWorkingMemoryStore(path string) *WorkingMemoryStore {
	return &WorkingMemoryStore{path: path}
}

// Load reads working memory from disk.
// Returns an empty initialized WorkingMemory if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *WorkingMemoryStore) Load() (*WorkingMemory, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WorkingMemory{
				Tasks:        make(map[string]*TaskMemory),
				Blockers:     []*BlockerMemory{},
				ContextHints: []string{},
			}, nil
		}
		return nil, err
	}

	var wm WorkingMemory
	if err := json.Unmarshal(data, &wm); err != nil {
		return nil, err
	}

	// Ensure non-nil maps and slices
	if wm.Tasks == nil {
		wm.Tasks = make(map[string]*TaskMemory)
	}
	if wm.Blockers == nil {
		wm.Blockers = []*BlockerMemory{}
	}
	if wm.ContextHints == nil {
		wm.ContextHints = []string{}
	}

	return &wm, nil
}

// Save writes working memory to disk atomically.
// Creates the file and any parent directories if they do not exist.
// Uses write-to-temp + rename for atomicity.
func (s *WorkingMemoryStore) Save(wm *WorkingMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wm.LastScanned = time.Now()

	data, err := json.MarshalIndent(wm, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "working-memory-*.json")
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

// AddOrUpdateTask adds a new task or merges data into an existing task.
// Merges sessions, blockers, and commits if the task already exists.
// Updates LastUpdated timestamp.
func (s *WorkingMemoryStore) AddOrUpdateTask(task *TaskMemory) error {
	wm, err := s.Load()
	if err != nil {
		return err
	}

	existing, exists := wm.Tasks[task.TaskIdentifier]
	if exists {
		// Merge sessions (deduplicate)
		sessionSet := make(map[string]bool)
		for _, sid := range existing.Sessions {
			sessionSet[sid] = true
		}
		for _, sid := range task.Sessions {
			sessionSet[sid] = true
		}
		mergedSessions := make([]string, 0, len(sessionSet))
		for sid := range sessionSet {
			mergedSessions = append(mergedSessions, sid)
		}
		existing.Sessions = mergedSessions

		// Merge blockers (deduplicate)
		blockerSet := make(map[string]bool)
		for _, b := range existing.BlockersHit {
			blockerSet[b] = true
		}
		for _, b := range task.BlockersHit {
			blockerSet[b] = true
		}
		mergedBlockers := make([]string, 0, len(blockerSet))
		for b := range blockerSet {
			mergedBlockers = append(mergedBlockers, b)
		}
		existing.BlockersHit = mergedBlockers

		// Merge commits (deduplicate)
		commitSet := make(map[string]bool)
		for _, c := range existing.Commits {
			commitSet[c] = true
		}
		for _, c := range task.Commits {
			commitSet[c] = true
		}
		mergedCommits := make([]string, 0, len(commitSet))
		for c := range commitSet {
			mergedCommits = append(mergedCommits, c)
		}
		existing.Commits = mergedCommits

		// Update other fields
		existing.Status = task.Status
		if task.Solution != "" {
			existing.Solution = task.Solution
		}
		existing.LastUpdated = time.Now()
	} else {
		task.LastUpdated = time.Now()
		wm.Tasks[task.TaskIdentifier] = task
	}

	return s.Save(wm)
}

// AddBlocker adds a new blocker or updates an existing one.
// Deduplicates by Issue field (case-insensitive match).
// Updates Encountered dates and LastSeen timestamp.
func (s *WorkingMemoryStore) AddBlocker(blocker *BlockerMemory) error {
	wm, err := s.Load()
	if err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")
	blocker.LastSeen = time.Now()

	// Check for existing blocker (case-insensitive Issue match)
	found := false
	for _, existing := range wm.Blockers {
		if strings.EqualFold(existing.Issue, blocker.Issue) {
			// Update existing blocker
			existing.LastSeen = blocker.LastSeen
			if blocker.File != "" {
				existing.File = blocker.File
			}
			if blocker.Solution != "" {
				existing.Solution = blocker.Solution
			}

			// Add today's date if not already present
			dateExists := false
			for _, date := range existing.Encountered {
				if date == today {
					dateExists = true
					break
				}
			}
			if !dateExists {
				existing.Encountered = append(existing.Encountered, today)
			}

			found = true
			break
		}
	}

	if !found {
		// Add new blocker
		if blocker.Encountered == nil {
			blocker.Encountered = []string{today}
		}
		wm.Blockers = append(wm.Blockers, blocker)
	}

	return s.Save(wm)
}

// GetTaskHistory returns tasks matching the given substring (case-insensitive).
// Returns all tasks if taskSubstring is empty.
func (s *WorkingMemoryStore) GetTaskHistory(taskSubstring string) ([]*TaskMemory, error) {
	wm, err := s.Load()
	if err != nil {
		return nil, err
	}

	var results []*TaskMemory
	for _, task := range wm.Tasks {
		if taskSubstring == "" || strings.Contains(strings.ToLower(task.TaskIdentifier), strings.ToLower(taskSubstring)) {
			results = append(results, task)
		}
	}

	return results, nil
}

// GetRecentBlockers returns blockers seen within the last N days.
// Returns all blockers if days is 0 or negative.
func (s *WorkingMemoryStore) GetRecentBlockers(days int) ([]*BlockerMemory, error) {
	wm, err := s.Load()
	if err != nil {
		return nil, err
	}

	if days <= 0 {
		return wm.Blockers, nil
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var results []*BlockerMemory
	for _, blocker := range wm.Blockers {
		if blocker.LastSeen.After(cutoff) || blocker.LastSeen.Equal(cutoff) {
			results = append(results, blocker)
		}
	}

	return results, nil
}
