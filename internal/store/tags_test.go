package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

func TestNewSessionTagStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "tags.json")

	s := store.NewSessionTagStore(path)
	tags, err := s.Load()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if tags == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(tags))
	}
}

func TestSessionTagStore_SetAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tags.json")

	s := store.NewSessionTagStore(path)

	if err := s.Set("session-abc", "myproject"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	tags, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got, ok := tags["session-abc"]; !ok {
		t.Fatal("expected session-abc in tags, not found")
	} else if got != "myproject" {
		t.Fatalf("expected 'myproject', got %q", got)
	}
}

func TestSessionTagStore_SetMultiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tags.json")

	s := store.NewSessionTagStore(path)

	entries := map[string]string{
		"session-1": "project-alpha",
		"session-2": "project-beta",
		"session-3": "project-gamma",
	}

	for sessionID, projectName := range entries {
		if err := s.Set(sessionID, projectName); err != nil {
			t.Fatalf("Set(%q, %q) failed: %v", sessionID, projectName, err)
		}
	}

	tags, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	for sessionID, expectedProject := range entries {
		if got, ok := tags[sessionID]; !ok {
			t.Errorf("expected %q in tags, not found", sessionID)
		} else if got != expectedProject {
			t.Errorf("tags[%q]: expected %q, got %q", sessionID, expectedProject, got)
		}
	}
}

func TestSessionTagStore_SetOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tags.json")

	s := store.NewSessionTagStore(path)

	if err := s.Set("session-xyz", "first-project"); err != nil {
		t.Fatalf("first Set failed: %v", err)
	}

	if err := s.Set("session-xyz", "second-project"); err != nil {
		t.Fatalf("second Set failed: %v", err)
	}

	tags, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got, ok := tags["session-xyz"]; !ok {
		t.Fatal("expected session-xyz in tags, not found")
	} else if got != "second-project" {
		t.Fatalf("expected 'second-project' after overwrite, got %q", got)
	}

	if len(tags) != 1 {
		t.Fatalf("expected exactly 1 tag, got %d", len(tags))
	}
}

func TestSessionTagStore_LoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tags.json")

	if err := os.WriteFile(path, []byte("this is not valid json {{{"), 0o644); err != nil {
		t.Fatalf("failed to write invalid JSON file: %v", err)
	}

	s := store.NewSessionTagStore(path)
	tags, err := s.Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if tags != nil {
		t.Fatalf("expected nil map on error, got %v", tags)
	}
}
