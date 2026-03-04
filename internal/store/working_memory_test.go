package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWorkingMemoryStore_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)
	wm, err := store.Load()
	if err != nil {
		t.Fatalf("Load on non-existent file should not error, got: %v", err)
	}

	if wm == nil {
		t.Fatal("Load should return non-nil WorkingMemory")
	}
	if wm.Tasks == nil {
		t.Fatal("WorkingMemory.Tasks should be initialized")
	}
	if wm.Blockers == nil {
		t.Fatal("WorkingMemory.Blockers should be initialized")
	}
	if wm.ContextHints == nil {
		t.Fatal("WorkingMemory.ContextHints should be initialized")
	}
	if len(wm.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(wm.Tasks))
	}
	if len(wm.Blockers) != 0 {
		t.Errorf("Expected 0 blockers, got %d", len(wm.Blockers))
	}
}

func TestWorkingMemoryStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Create test data
	now := time.Now().Truncate(time.Second) // Truncate for JSON round-trip
	wm := &WorkingMemory{
		Tasks: map[string]*TaskMemory{
			"test-task": {
				TaskIdentifier: "test-task",
				Sessions:       []string{"session1", "session2"},
				Status:         "completed",
				BlockersHit:    []string{"blocker1"},
				Solution:       "fixed it",
				Commits:        []string{"abc123"},
				LastUpdated:    now,
			},
		},
		Blockers: []*BlockerMemory{
			{
				File:        "main.go",
				Issue:       "compilation error",
				Solution:    "fix syntax",
				Encountered: []string{"2026-01-01"},
				LastSeen:    now,
			},
		},
		ContextHints: []string{"config.yaml", "main.go"},
		LastScanned:  now,
	}

	// Save
	if err := store.Save(wm); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify tasks
	if len(loaded.Tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(loaded.Tasks))
	}
	task := loaded.Tasks["test-task"]
	if task == nil {
		t.Fatal("Task 'test-task' not found")
	}
	if task.TaskIdentifier != "test-task" {
		t.Errorf("Expected TaskIdentifier 'test-task', got '%s'", task.TaskIdentifier)
	}
	if len(task.Sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(task.Sessions))
	}
	if task.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", task.Status)
	}
	if task.Solution != "fixed it" {
		t.Errorf("Expected solution 'fixed it', got '%s'", task.Solution)
	}

	// Verify blockers
	if len(loaded.Blockers) != 1 {
		t.Fatalf("Expected 1 blocker, got %d", len(loaded.Blockers))
	}
	blocker := loaded.Blockers[0]
	if blocker.File != "main.go" {
		t.Errorf("Expected File 'main.go', got '%s'", blocker.File)
	}
	if blocker.Issue != "compilation error" {
		t.Errorf("Expected Issue 'compilation error', got '%s'", blocker.Issue)
	}

	// Verify context hints
	if len(loaded.ContextHints) != 2 {
		t.Errorf("Expected 2 context hints, got %d", len(loaded.ContextHints))
	}
}

func TestWorkingMemoryStore_AddOrUpdateTask_New(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	task := &TaskMemory{
		TaskIdentifier: "new-task",
		Sessions:       []string{"session1"},
		Status:         "in_progress",
		BlockersHit:    []string{},
		Commits:        []string{},
	}

	if err := store.AddOrUpdateTask(task); err != nil {
		t.Fatalf("AddOrUpdateTask failed: %v", err)
	}

	// Load and verify
	wm, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(wm.Tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(wm.Tasks))
	}
	loaded := wm.Tasks["new-task"]
	if loaded == nil {
		t.Fatal("Task 'new-task' not found")
	}
	if loaded.Status != "in_progress" {
		t.Errorf("Expected status 'in_progress', got '%s'", loaded.Status)
	}
	if loaded.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set")
	}
}

func TestWorkingMemoryStore_AddOrUpdateTask_Merge(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Add initial task
	task1 := &TaskMemory{
		TaskIdentifier: "merge-task",
		Sessions:       []string{"session1"},
		Status:         "in_progress",
		BlockersHit:    []string{"blocker1"},
		Commits:        []string{"commit1"},
	}
	if err := store.AddOrUpdateTask(task1); err != nil {
		t.Fatalf("AddOrUpdateTask failed: %v", err)
	}

	// Update with overlapping and new data
	task2 := &TaskMemory{
		TaskIdentifier: "merge-task",
		Sessions:       []string{"session1", "session2"}, // session1 overlaps
		Status:         "completed",
		BlockersHit:    []string{"blocker2"}, // new blocker
		Solution:       "fixed",
		Commits:        []string{"commit1", "commit2"}, // commit1 overlaps
	}
	if err := store.AddOrUpdateTask(task2); err != nil {
		t.Fatalf("AddOrUpdateTask failed: %v", err)
	}

	// Load and verify merge
	wm, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	task := wm.Tasks["merge-task"]
	if task == nil {
		t.Fatal("Task 'merge-task' not found")
	}

	// Verify sessions merged (should have 2 unique)
	if len(task.Sessions) != 2 {
		t.Errorf("Expected 2 sessions after merge, got %d", len(task.Sessions))
	}
	sessionMap := make(map[string]bool)
	for _, s := range task.Sessions {
		sessionMap[s] = true
	}
	if !sessionMap["session1"] || !sessionMap["session2"] {
		t.Error("Expected both session1 and session2 in merged sessions")
	}

	// Verify blockers merged (should have 2 unique)
	if len(task.BlockersHit) != 2 {
		t.Errorf("Expected 2 blockers after merge, got %d", len(task.BlockersHit))
	}

	// Verify commits merged (should have 2 unique)
	if len(task.Commits) != 2 {
		t.Errorf("Expected 2 commits after merge, got %d", len(task.Commits))
	}

	// Verify status updated
	if task.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", task.Status)
	}

	// Verify solution updated
	if task.Solution != "fixed" {
		t.Errorf("Expected solution 'fixed', got '%s'", task.Solution)
	}
}

func TestWorkingMemoryStore_AddBlocker_New(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	blocker := &BlockerMemory{
		File:     "test.go",
		Issue:    "test error",
		Solution: "test fix",
	}

	if err := store.AddBlocker(blocker); err != nil {
		t.Fatalf("AddBlocker failed: %v", err)
	}

	// Load and verify
	wm, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(wm.Blockers) != 1 {
		t.Fatalf("Expected 1 blocker, got %d", len(wm.Blockers))
	}

	loaded := wm.Blockers[0]
	if loaded.Issue != "test error" {
		t.Errorf("Expected Issue 'test error', got '%s'", loaded.Issue)
	}
	if loaded.LastSeen.IsZero() {
		t.Error("LastSeen should be set")
	}
	if len(loaded.Encountered) != 1 {
		t.Errorf("Expected 1 encountered date, got %d", len(loaded.Encountered))
	}
}

func TestWorkingMemoryStore_AddBlocker_Deduplicate(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Add first blocker
	blocker1 := &BlockerMemory{
		File:     "old.go",
		Issue:    "Test Error",
		Solution: "old fix",
	}
	if err := store.AddBlocker(blocker1); err != nil {
		t.Fatalf("AddBlocker failed: %v", err)
	}

	// Add duplicate (case-insensitive match on Issue)
	blocker2 := &BlockerMemory{
		File:     "new.go",
		Issue:    "test error", // Different case
		Solution: "new fix",
	}
	if err := store.AddBlocker(blocker2); err != nil {
		t.Fatalf("AddBlocker failed: %v", err)
	}

	// Load and verify deduplication
	wm, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(wm.Blockers) != 1 {
		t.Fatalf("Expected 1 blocker (deduplicated), got %d", len(wm.Blockers))
	}

	blocker := wm.Blockers[0]
	// Should have updated file and solution
	if blocker.File != "new.go" {
		t.Errorf("Expected File 'new.go', got '%s'", blocker.File)
	}
	if blocker.Solution != "new fix" {
		t.Errorf("Expected Solution 'new fix', got '%s'", blocker.Solution)
	}

	// Encountered dates should contain today's date
	today := time.Now().Format("2006-01-02")
	if len(blocker.Encountered) != 1 {
		t.Errorf("Expected 1 encountered date, got %d", len(blocker.Encountered))
	}
	if blocker.Encountered[0] != today {
		t.Errorf("Expected encountered date '%s', got '%s'", today, blocker.Encountered[0])
	}
}

func TestWorkingMemoryStore_GetTaskHistory_SubstringMatch(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Add multiple tasks
	tasks := []*TaskMemory{
		{TaskIdentifier: "feature-auth", Status: "completed"},
		{TaskIdentifier: "feature-logging", Status: "in_progress"},
		{TaskIdentifier: "bugfix-auth", Status: "completed"},
		{TaskIdentifier: "refactor-database", Status: "abandoned"},
	}

	for _, task := range tasks {
		if err := store.AddOrUpdateTask(task); err != nil {
			t.Fatalf("AddOrUpdateTask failed: %v", err)
		}
	}

	// Test case-insensitive substring match
	results, err := store.GetTaskHistory("AUTH")
	if err != nil {
		t.Fatalf("GetTaskHistory failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results for 'AUTH', got %d", len(results))
	}

	// Verify both auth-related tasks are returned
	foundFeature := false
	foundBugfix := false
	for _, task := range results {
		if task.TaskIdentifier == "feature-auth" {
			foundFeature = true
		}
		if task.TaskIdentifier == "bugfix-auth" {
			foundBugfix = true
		}
	}
	if !foundFeature || !foundBugfix {
		t.Error("Expected both 'feature-auth' and 'bugfix-auth' in results")
	}
}

func TestWorkingMemoryStore_GetTaskHistory_EmptySubstring(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Add tasks
	tasks := []*TaskMemory{
		{TaskIdentifier: "task1", Status: "completed"},
		{TaskIdentifier: "task2", Status: "in_progress"},
	}

	for _, task := range tasks {
		if err := store.AddOrUpdateTask(task); err != nil {
			t.Fatalf("AddOrUpdateTask failed: %v", err)
		}
	}

	// Empty substring should return all tasks
	results, err := store.GetTaskHistory("")
	if err != nil {
		t.Fatalf("GetTaskHistory failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for empty substring, got %d", len(results))
	}
}

func TestWorkingMemoryStore_GetRecentBlockers_DateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "working-memory.json")

	store := NewWorkingMemoryStore(storePath)

	// Add blockers with different LastSeen dates
	now := time.Now()
	blockers := []*BlockerMemory{
		{
			Issue:    "recent blocker",
			LastSeen: now.AddDate(0, 0, -1), // 1 day ago
		},
		{
			Issue:    "old blocker",
			LastSeen: now.AddDate(0, 0, -10), // 10 days ago
		},
		{
			Issue:    "very old blocker",
			LastSeen: now.AddDate(0, 0, -100), // 100 days ago
		},
	}

	// Manually save to set specific LastSeen dates
	wm := &WorkingMemory{
		Tasks:        make(map[string]*TaskMemory),
		Blockers:     blockers,
		ContextHints: []string{},
	}
	if err := store.Save(wm); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test: Get blockers from last 7 days
	results, err := store.GetRecentBlockers(7)
	if err != nil {
		t.Fatalf("GetRecentBlockers failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 recent blocker (within 7 days), got %d", len(results))
	}
	if results[0].Issue != "recent blocker" {
		t.Errorf("Expected 'recent blocker', got '%s'", results[0].Issue)
	}

	// Test: Get blockers from last 30 days
	results, err = store.GetRecentBlockers(30)
	if err != nil {
		t.Fatalf("GetRecentBlockers failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 blockers (within 30 days), got %d", len(results))
	}

	// Test: days <= 0 returns all blockers
	results, err = store.GetRecentBlockers(0)
	if err != nil {
		t.Fatalf("GetRecentBlockers failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 blockers (all), got %d", len(results))
	}
}
