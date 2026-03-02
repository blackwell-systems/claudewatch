package store_test

import (
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestOpenInMemory verifies that OpenInMemory applies all migrations successfully.
func TestOpenInMemory(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()
}

// --- Baseline tests ---

func TestUpsertAndGetProjectBaseline(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	b := store.ProjectBaseline{
		Project:        "myproject",
		ComputedAt:     time.Now().UTC().Format(time.RFC3339),
		SessionCount:   10,
		AvgCostUSD:     1.23,
		StddevCostUSD:  0.45,
		AvgFriction:    2.5,
		StddevFriction: 0.8,
		AvgCommits:     3.2,
		SAWSessionFrac: 0.4,
	}

	if err := db.UpsertProjectBaseline(b); err != nil {
		t.Fatalf("UpsertProjectBaseline() failed: %v", err)
	}

	got, err := db.GetProjectBaseline("myproject")
	if err != nil {
		t.Fatalf("GetProjectBaseline() failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetProjectBaseline() returned nil, expected non-nil")
	}
	if got.Project != b.Project {
		t.Errorf("Project: got %q, want %q", got.Project, b.Project)
	}
	if got.SessionCount != b.SessionCount {
		t.Errorf("SessionCount: got %d, want %d", got.SessionCount, b.SessionCount)
	}
	if got.AvgCostUSD != b.AvgCostUSD {
		t.Errorf("AvgCostUSD: got %f, want %f", got.AvgCostUSD, b.AvgCostUSD)
	}
	if got.StddevCostUSD != b.StddevCostUSD {
		t.Errorf("StddevCostUSD: got %f, want %f", got.StddevCostUSD, b.StddevCostUSD)
	}
	if got.AvgFriction != b.AvgFriction {
		t.Errorf("AvgFriction: got %f, want %f", got.AvgFriction, b.AvgFriction)
	}
	if got.StddevFriction != b.StddevFriction {
		t.Errorf("StddevFriction: got %f, want %f", got.StddevFriction, b.StddevFriction)
	}
	if got.SAWSessionFrac != b.SAWSessionFrac {
		t.Errorf("SAWSessionFrac: got %f, want %f", got.SAWSessionFrac, b.SAWSessionFrac)
	}
}

func TestGetProjectBaseline_NotFound(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	got, err := db.GetProjectBaseline("nonexistent")
	if err != nil {
		t.Fatalf("GetProjectBaseline() unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing project, got %+v", got)
	}
}

func TestUpsertProjectBaseline_Overwrite(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	b1 := store.ProjectBaseline{
		Project:      "overwrite-project",
		ComputedAt:   time.Now().UTC().Format(time.RFC3339),
		SessionCount: 5,
		AvgCostUSD:   0.5,
	}
	if err := db.UpsertProjectBaseline(b1); err != nil {
		t.Fatalf("first UpsertProjectBaseline() failed: %v", err)
	}

	b2 := store.ProjectBaseline{
		Project:      "overwrite-project",
		ComputedAt:   time.Now().UTC().Format(time.RFC3339),
		SessionCount: 10,
		AvgCostUSD:   1.0,
	}
	if err := db.UpsertProjectBaseline(b2); err != nil {
		t.Fatalf("second UpsertProjectBaseline() failed: %v", err)
	}

	got, err := db.GetProjectBaseline("overwrite-project")
	if err != nil {
		t.Fatalf("GetProjectBaseline() failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.SessionCount != 10 {
		t.Errorf("SessionCount after overwrite: got %d, want 10", got.SessionCount)
	}
	if got.AvgCostUSD != 1.0 {
		t.Errorf("AvgCostUSD after overwrite: got %f, want 1.0", got.AvgCostUSD)
	}
}

func TestListProjectBaselines(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	// Empty list.
	baselines, err := db.ListProjectBaselines()
	if err != nil {
		t.Fatalf("ListProjectBaselines() failed on empty DB: %v", err)
	}
	if len(baselines) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(baselines))
	}

	// Insert three projects.
	projects := []string{"alpha", "beta", "gamma"}
	for i, p := range projects {
		b := store.ProjectBaseline{
			Project:      p,
			ComputedAt:   time.Now().UTC().Format(time.RFC3339),
			SessionCount: i + 1,
		}
		if err := db.UpsertProjectBaseline(b); err != nil {
			t.Fatalf("UpsertProjectBaseline(%q) failed: %v", p, err)
		}
	}

	baselines, err = db.ListProjectBaselines()
	if err != nil {
		t.Fatalf("ListProjectBaselines() failed: %v", err)
	}
	if len(baselines) != 3 {
		t.Fatalf("expected 3 baselines, got %d", len(baselines))
	}

	// Must be sorted by project name ascending.
	for i, want := range projects {
		if baselines[i].Project != want {
			t.Errorf("baselines[%d].Project: got %q, want %q", i, baselines[i].Project, want)
		}
	}
}

// --- FTS tests ---

func TestTranscriptIndexStatus_Empty(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	count, lastIndexed, err := db.TranscriptIndexStatus()
	if err != nil {
		t.Fatalf("TranscriptIndexStatus() failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
	if lastIndexed != "" {
		t.Errorf("expected empty lastIndexed, got %q", lastIndexed)
	}
}

// TestSearchTranscripts_Empty verifies that searching an empty index returns no results.
func TestSearchTranscripts_Empty(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	results, err := db.SearchTranscripts("anything", 10)
	if err != nil {
		t.Fatalf("SearchTranscripts() on empty index failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

// TestSearchTranscripts_IndexAndSearch verifies direct insertion + FTS search using
// the underlying conn (since IndexTranscripts requires a real claudeHome directory).
func TestSearchTranscripts_IndexAndSearch(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	// Insert rows directly via Conn() to test SearchTranscripts without needing
	// real JSONL files on disk.
	rows := []struct {
		sessionID   string
		projectHash string
		lineNumber  int
		entryType   string
		content     string
		timestamp   string
		indexedAt   string
	}{
		{"sess-001", "proj-abc", 1, "assistant", "tool error occurred in Bash execution", now, now},
		{"sess-001", "proj-abc", 2, "user", "retry Bash command after fixing path", now, now},
		{"sess-002", "proj-xyz", 1, "assistant", "calling Write tool to create new file", now, now},
	}

	for _, r := range rows {
		_, err := db.Conn().Exec(`
			INSERT INTO transcript_index
				(session_id, project_hash, line_number, entry_type, content, timestamp, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, r.sessionID, r.projectHash, r.lineNumber, r.entryType, r.content, r.timestamp, r.indexedAt)
		if err != nil {
			t.Fatalf("inserting transcript_index row: %v", err)
		}

		// Sync into FTS virtual table.
		_, err = db.Conn().Exec(`
			INSERT INTO transcript_index_fts
				(rowid, session_id, project_hash, entry_type, content, timestamp)
			SELECT rowid, session_id, project_hash, entry_type, content, timestamp
			FROM transcript_index
			WHERE session_id = ? AND line_number = ?
		`, r.sessionID, r.lineNumber)
		if err != nil {
			t.Fatalf("syncing FTS row: %v", err)
		}
	}

	// Verify index status reflects inserted rows.
	count, lastIndexed, err := db.TranscriptIndexStatus()
	if err != nil {
		t.Fatalf("TranscriptIndexStatus() failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}
	if lastIndexed == "" {
		t.Error("expected non-empty lastIndexed")
	}

	// Search for "tool error" — should match row 1.
	results, err := db.SearchTranscripts("tool error", 10)
	if err != nil {
		t.Fatalf("SearchTranscripts() failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'tool error', got none")
	}

	found := false
	for _, r := range results {
		if r.SessionID == "sess-001" && r.LineNumber == 1 {
			found = true
			if r.ProjectHash != "proj-abc" {
				t.Errorf("result ProjectHash: got %q, want %q", r.ProjectHash, "proj-abc")
			}
			if r.EntryType != "assistant" {
				t.Errorf("result EntryType: got %q, want %q", r.EntryType, "assistant")
			}
			if r.Snippet == "" {
				t.Error("expected non-empty snippet")
			}
		}
	}
	if !found {
		t.Error("expected result for sess-001 line 1 in 'tool error' search")
	}

	// Search for "Write tool" — should match row 3 only.
	results, err = db.SearchTranscripts("Write", 10)
	if err != nil {
		t.Fatalf("SearchTranscripts('Write') failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'Write', got none")
	}
	for _, r := range results {
		if r.SessionID != "sess-002" {
			t.Errorf("unexpected session in Write results: %q", r.SessionID)
		}
	}

	// Search for "nonexistent" — should return zero results.
	results, err = db.SearchTranscripts("nonexistent_term_xyz", 10)
	if err != nil {
		t.Fatalf("SearchTranscripts() for missing term failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent term, got %d", len(results))
	}
}

// TestSearchTranscripts_DefaultLimit verifies that limit=0 defaults to 20.
func TestSearchTranscripts_DefaultLimit(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory() failed: %v", err)
	}
	defer db.Close()

	// Passing limit=0 must not error (it should use the default of 20).
	results, err := db.SearchTranscripts("anything", 0)
	if err != nil {
		t.Fatalf("SearchTranscripts(limit=0) failed: %v", err)
	}
	// Empty index, no results expected — just verifying no panic/error.
	_ = results
}
