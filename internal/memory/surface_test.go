package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected []string
	}{
		{
			name:     "simple message with stop words",
			message:  "implement authentication for the API",
			expected: []string{"implement", "authentication", "api"},
		},
		{
			name:     "message with punctuation",
			message:  "Fix bug in database connection, please!",
			expected: []string{"fix", "bug", "database", "connection", "please"},
		},
		{
			name:     "short words filtered",
			message:  "add a new UI to app",
			expected: []string{"add", "new", "app"},
		},
		{
			name:     "duplicate keywords deduplicated",
			message:  "test the test suite tests",
			expected: []string{"test", "suite", "tests"},
		},
		{
			name:     "empty message",
			message:  "",
			expected: []string{},
		},
		{
			name:     "only stop words",
			message:  "the a an and or is to of",
			expected: []string{},
		},
		{
			name:     "case insensitive",
			message:  "Authentication AUTH authentication",
			expected: []string{"authentication", "auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeywords(tt.message)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d keywords, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, kw := range tt.expected {
				if result[i] != kw {
					t.Errorf("keyword[%d]: expected %q, got %q", i, kw, result[i])
				}
			}
		})
	}
}

func TestSurfaceRelevantMemory(t *testing.T) {
	// Create temporary directory for test store.
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	// Populate with test tasks.
	task1 := &store.TaskMemory{
		TaskIdentifier: "implement authentication system",
		Sessions:       []string{"session-1"},
		Status:         "completed",
		BlockersHit:    []string{"CORS issue"},
		Solution:       "Added CORS headers to middleware",
		Commits:        []string{"abc123"},
		LastUpdated:    time.Now().Add(-24 * time.Hour),
	}

	task2 := &store.TaskMemory{
		TaskIdentifier: "fix database connection timeout",
		Sessions:       []string{"session-2"},
		Status:         "abandoned",
		BlockersHit:    []string{"Network latency too high"},
		Solution:       "",
		Commits:        []string{},
		LastUpdated:    time.Now().Add(-48 * time.Hour),
	}

	task3 := &store.TaskMemory{
		TaskIdentifier: "refactor api endpoints",
		Sessions:       []string{"session-3"},
		Status:         "completed",
		BlockersHit:    []string{},
		Solution:       "Split monolithic handler into smaller functions",
		Commits:        []string{"def456"},
		LastUpdated:    time.Now().Add(-12 * time.Hour),
	}

	if err := memStore.AddOrUpdateTask(task1); err != nil {
		t.Fatalf("failed to add task1: %v", err)
	}
	if err := memStore.AddOrUpdateTask(task2); err != nil {
		t.Fatalf("failed to add task2: %v", err)
	}
	if err := memStore.AddOrUpdateTask(task3); err != nil {
		t.Fatalf("failed to add task3: %v", err)
	}

	tests := []struct {
		name          string
		userMessage   string
		expectedCount int
		expectedTasks []string // TaskIdentifier substrings to match
	}{
		{
			name:          "match authentication",
			userMessage:   "implement authentication for users",
			expectedCount: 1,
			expectedTasks: []string{"implement authentication system"},
		},
		{
			name:          "match multiple keywords",
			userMessage:   "fix the API authentication issue",
			expectedCount: 3,
			expectedTasks: []string{"implement authentication system", "refactor api endpoints", "fix database connection timeout"},
		},
		{
			name:          "no match",
			userMessage:   "create frontend dashboard",
			expectedCount: 0,
			expectedTasks: []string{},
		},
		{
			name:          "match database",
			userMessage:   "investigate database performance",
			expectedCount: 1,
			expectedTasks: []string{"fix database connection timeout"},
		},
		{
			name:          "empty message",
			userMessage:   "",
			expectedCount: 0,
			expectedTasks: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SurfaceRelevantMemory(tt.userMessage, "test-project", memStore)
			if err != nil {
				t.Fatalf("SurfaceRelevantMemory failed: %v", err)
			}

			if len(result.MatchedTasks) != tt.expectedCount {
				t.Errorf("expected %d matched tasks, got %d", tt.expectedCount, len(result.MatchedTasks))
			}

			// Verify expected task identifiers are present.
			for _, expectedID := range tt.expectedTasks {
				found := false
				for _, task := range result.MatchedTasks {
					if strings.Contains(task.TaskIdentifier, expectedID) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected task %q not found in results", expectedID)
				}
			}

			// Verify tasks are sorted by LastUpdated descending (most recent first).
			if len(result.MatchedTasks) > 1 {
				for i := 0; i < len(result.MatchedTasks)-1; i++ {
					if result.MatchedTasks[i].LastUpdated.Before(result.MatchedTasks[i+1].LastUpdated) {
						t.Errorf("tasks not sorted by LastUpdated descending: %v before %v",
							result.MatchedTasks[i].LastUpdated, result.MatchedTasks[i+1].LastUpdated)
					}
				}
			}
		})
	}

	// Cleanup.
	_ = os.Remove(storePath)
}

func TestSurfaceRelevantMemory_NoStore(t *testing.T) {
	// Test with non-existent store (should not error, just return empty result).
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "nonexistent.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	result, err := SurfaceRelevantMemory("implement authentication", "test-project", memStore)
	if err != nil {
		t.Fatalf("expected no error with empty store, got: %v", err)
	}

	if len(result.MatchedTasks) != 0 {
		t.Errorf("expected 0 matched tasks, got %d", len(result.MatchedTasks))
	}
}
