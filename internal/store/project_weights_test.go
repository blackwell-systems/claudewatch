package store

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleWeights() []ProjectWeight {
	return []ProjectWeight{
		{Project: "myproject", RepoRoot: "/home/user/myproject", Weight: 0.75, ToolCalls: 30},
		{Project: "otherproject", RepoRoot: "/home/user/otherproject", Weight: 0.25, ToolCalls: 10},
	}
}

func TestSessionProjectWeightsStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	m, err := s.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(m))
	}
}

func TestSessionProjectWeightsStore_SetAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	ws := sampleWeights()
	if err := s.Set("session-1", ws); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	m, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got, ok := m["session-1"]
	if !ok {
		t.Fatal("expected session-1 in loaded map")
	}
	if len(got) != len(ws) {
		t.Fatalf("expected %d weights, got %d", len(ws), len(got))
	}
	for i, w := range ws {
		if got[i].Project != w.Project {
			t.Errorf("weight[%d].Project: want %q, got %q", i, w.Project, got[i].Project)
		}
		if got[i].RepoRoot != w.RepoRoot {
			t.Errorf("weight[%d].RepoRoot: want %q, got %q", i, w.RepoRoot, got[i].RepoRoot)
		}
		if got[i].Weight != w.Weight {
			t.Errorf("weight[%d].Weight: want %f, got %f", i, w.Weight, got[i].Weight)
		}
		if got[i].ToolCalls != w.ToolCalls {
			t.Errorf("weight[%d].ToolCalls: want %d, got %d", i, w.ToolCalls, got[i].ToolCalls)
		}
	}
}

func TestSessionProjectWeightsStore_SetMultipleSessions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	ws1 := []ProjectWeight{{Project: "alpha", RepoRoot: "/alpha", Weight: 1.0, ToolCalls: 5}}
	ws2 := []ProjectWeight{{Project: "beta", RepoRoot: "/beta", Weight: 0.5, ToolCalls: 3}}

	if err := s.Set("session-A", ws1); err != nil {
		t.Fatalf("Set session-A failed: %v", err)
	}
	if err := s.Set("session-B", ws2); err != nil {
		t.Fatalf("Set session-B failed: %v", err)
	}

	m, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(m) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(m))
	}
	if _, ok := m["session-A"]; !ok {
		t.Error("session-A missing from map")
	}
	if _, ok := m["session-B"]; !ok {
		t.Error("session-B missing from map")
	}
}

func TestSessionProjectWeightsStore_SetOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	first := []ProjectWeight{{Project: "first", RepoRoot: "/first", Weight: 1.0, ToolCalls: 1}}
	second := []ProjectWeight{{Project: "second", RepoRoot: "/second", Weight: 0.5, ToolCalls: 2}}

	if err := s.Set("session-1", first); err != nil {
		t.Fatalf("first Set failed: %v", err)
	}
	if err := s.Set("session-1", second); err != nil {
		t.Fatalf("second Set failed: %v", err)
	}

	m, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got := m["session-1"]
	if len(got) != 1 {
		t.Fatalf("expected 1 weight entry, got %d", len(got))
	}
	if got[0].Project != "second" {
		t.Errorf("expected project %q, got %q", "second", got[0].Project)
	}
}

func TestSessionProjectWeightsStore_GetWeights(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	ws := sampleWeights()
	if err := s.Set("session-1", ws); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := s.GetWeights("session-1")
	if err != nil {
		t.Fatalf("GetWeights failed: %v", err)
	}
	if len(got) != len(ws) {
		t.Fatalf("expected %d weights, got %d", len(ws), len(got))
	}
	if got[0].Project != ws[0].Project {
		t.Errorf("expected project %q, got %q", ws[0].Project, got[0].Project)
	}
}

func TestSessionProjectWeightsStore_GetWeightsNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	got, err := s.GetWeights("nonexistent-session")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown session, got %v", got)
	}
}

func TestSessionProjectWeightsStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	s := NewSessionProjectWeightsStore(path)

	ws := sampleWeights()
	if err := s.Set("session-1", ws); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after Set, got error: %v", err)
	}
}

func TestSessionProjectWeightsStore_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "weights.json")
	s := NewSessionProjectWeightsStore(path)

	ws := sampleWeights()
	if err := s.Set("session-1", ws); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at nested path to exist, got: %v", err)
	}
}
