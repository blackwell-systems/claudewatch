package mcp

import (
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// openTestDB opens the claudewatch DB at config.DBPath() (which must have been
// redirected to a temp dir via t.Setenv("HOME", ...)) and returns it.
// The caller is responsible for closing the DB.
func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	return db
}

// TestSearchTranscripts_MissingQuery verifies that omitting query returns an error.
func TestSearchTranscripts_MissingQuery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := newTestServer(dir, 0)
	addTranscriptTools(s)

	_, err := callTool(s, "search_transcripts", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing query, got nil")
	}
	if err.Error() != "query is required" {
		t.Errorf("error = %q, want %q", err.Error(), "query is required")
	}
}

// TestSearchTranscripts_EmptyQuery verifies that an empty query string returns an error.
func TestSearchTranscripts_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := newTestServer(dir, 0)
	addTranscriptTools(s)

	_, err := callTool(s, "search_transcripts", json.RawMessage(`{"query":""}`))
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
	if err.Error() != "query is required" {
		t.Errorf("error = %q, want %q", err.Error(), "query is required")
	}
}

// TestSearchTranscripts_EmptyIndex verifies that searching an empty index returns the correct error.
func TestSearchTranscripts_EmptyIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Open the DB (which runs migrations) but insert no transcript entries.
	db := openTestDB(t)
	_ = db.Close()

	s := newTestServer(dir, 0)
	addTranscriptTools(s)

	_, err := callTool(s, "search_transcripts", json.RawMessage(`{"query":"hello"}`))
	if err == nil {
		t.Fatal("expected error for empty index, got nil")
	}
	want := "transcript index is empty — run 'claudewatch search <query>' first to index transcripts, or the index will be built automatically on next search"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestSearchTranscripts_ReturnsResults verifies that search returns results when data is indexed.
func TestSearchTranscripts_ReturnsResults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Open the DB (runs migrations) then index some transcripts by inserting
	// directly into transcript_index and transcript_index_fts.
	db := openTestDB(t)

	// Insert a row into transcript_index.
	_, err := db.Conn().Exec(`
		INSERT INTO transcript_index
			(session_id, project_hash, line_number, entry_type, content, timestamp, indexed_at)
		VALUES
			('sess-abc', 'proj-xyz', 1, 'assistant', 'hello world test content', '2026-01-15T10:00:00Z', '2026-01-15T10:01:00Z')
	`)
	if err != nil {
		t.Fatalf("insert transcript_index: %v", err)
	}

	// Sync into FTS.
	_, err = db.Conn().Exec(`
		INSERT INTO transcript_index_fts
			(rowid, session_id, project_hash, entry_type, content, timestamp)
		SELECT rowid, session_id, project_hash, entry_type, content, timestamp
		FROM transcript_index
		WHERE session_id = 'sess-abc' AND line_number = 1
	`)
	if err != nil {
		t.Fatalf("insert transcript_index_fts: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addTranscriptTools(s)

	result, err := callTool(s, "search_transcripts", json.RawMessage(`{"query":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(TranscriptSearchMCPResult)
	if !ok {
		t.Fatalf("expected TranscriptSearchMCPResult, got %T", result)
	}

	if r.Count == 0 {
		t.Error("Count = 0, want > 0")
	}
	if len(r.Results) == 0 {
		t.Error("Results is empty, want at least 1 result")
	}
	if r.Indexed == 0 {
		t.Error("Indexed = 0, want > 0")
	}

	// Verify the first result has the expected session ID.
	if len(r.Results) > 0 && r.Results[0].SessionID != "sess-abc" {
		t.Errorf("Results[0].SessionID = %q, want %q", r.Results[0].SessionID, "sess-abc")
	}
}

// TestSearchTranscripts_LimitRespected verifies that the limit parameter is honored.
func TestSearchTranscripts_LimitRespected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	db := openTestDB(t)

	// Insert 5 transcript rows.
	for i := 1; i <= 5; i++ {
		_, err := db.Conn().Exec(`
			INSERT INTO transcript_index
				(session_id, project_hash, line_number, entry_type, content, timestamp, indexed_at)
			VALUES
				(?, 'proj-lim', ?, 'assistant', 'searchable keyword content', '2026-01-15T10:00:00Z', '2026-01-15T10:01:00Z')
		`, "sess-lim", i)
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}
	_, err := db.Conn().Exec(`
		INSERT INTO transcript_index_fts
			(rowid, session_id, project_hash, entry_type, content, timestamp)
		SELECT rowid, session_id, project_hash, entry_type, content, timestamp
		FROM transcript_index
		WHERE project_hash = 'proj-lim'
	`)
	if err != nil {
		t.Fatalf("sync FTS: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addTranscriptTools(s)

	// Request limit=2.
	result, err := callTool(s, "search_transcripts", json.RawMessage(`{"query":"keyword","limit":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(TranscriptSearchMCPResult)
	if !ok {
		t.Fatalf("expected TranscriptSearchMCPResult, got %T", result)
	}

	if len(r.Results) > 2 {
		t.Errorf("len(Results) = %d, want ≤ 2", len(r.Results))
	}
	if r.Count > 2 {
		t.Errorf("Count = %d, want ≤ 2", r.Count)
	}
}
