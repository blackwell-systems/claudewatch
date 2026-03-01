package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// newTagTestServer creates a Server with claudeHome and tagStorePath both
// pointing into a temp directory. It does NOT call addTools so tests remain focused.
func newTagTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return &Server{
		claudeHome:   dir,
		tagStorePath: filepath.Join(dir, "session-tags.json"),
	}
}

// TestHandleSetSessionProject_OK verifies that a valid call writes to the tag store and returns OK:true.
func TestHandleSetSessionProject_OK(t *testing.T) {
	s := newTagTestServer(t)

	args := json.RawMessage(`{"session_id":"sess-abc","project_name":"myproject"}`)
	result, err := s.handleSetSessionProject(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SetSessionProjectResult)
	if !ok {
		t.Fatalf("expected SetSessionProjectResult, got %T", result)
	}
	if !r.OK {
		t.Errorf("OK = false, want true")
	}
	if r.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-abc")
	}
	if r.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", r.ProjectName, "myproject")
	}

	// Verify the tag was actually written to the store.
	ts := store.NewSessionTagStore(s.tagStorePath)
	tags, err := ts.Load()
	if err != nil {
		t.Fatalf("failed to load tags: %v", err)
	}
	if tags["sess-abc"] != "myproject" {
		t.Errorf("stored tag = %q, want %q", tags["sess-abc"], "myproject")
	}
}

// TestHandleSetSessionProject_MissingSessionID verifies that missing session_id returns an error.
func TestHandleSetSessionProject_MissingSessionID(t *testing.T) {
	s := newTagTestServer(t)

	args := json.RawMessage(`{"project_name":"foo"}`)
	_, err := s.handleSetSessionProject(args)
	if err == nil {
		t.Fatal("expected error for missing session_id, got nil")
	}
	if !strings.Contains(err.Error(), "session_id") {
		t.Errorf("error %q should contain %q", err.Error(), "session_id")
	}
}

// TestHandleSetSessionProject_MissingProjectName verifies that missing project_name returns an error.
func TestHandleSetSessionProject_MissingProjectName(t *testing.T) {
	s := newTagTestServer(t)

	args := json.RawMessage(`{"session_id":"abc"}`)
	_, err := s.handleSetSessionProject(args)
	if err == nil {
		t.Fatal("expected error for missing project_name, got nil")
	}
	if !strings.Contains(err.Error(), "project_name") {
		t.Errorf("error %q should contain %q", err.Error(), "project_name")
	}
}

// TestHandleSetSessionProject_InvalidJSON verifies that malformed JSON returns an error.
func TestHandleSetSessionProject_InvalidJSON(t *testing.T) {
	s := newTagTestServer(t)

	args := json.RawMessage(`{invalid json`)
	_, err := s.handleSetSessionProject(args)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
