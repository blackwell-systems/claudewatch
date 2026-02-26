package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListProjects_MultipleProjects(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(filepath.Join(projDir, "hash-aaa"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projDir, "hash-bbb"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	projects, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	names := map[string]bool{}
	for _, p := range projects {
		names[p.Name] = true
	}
	if !names["hash-aaa"] || !names["hash-bbb"] {
		t.Errorf("expected hash-aaa and hash-bbb, got %v", names)
	}
}

func TestListProjects_MissingDir(t *testing.T) {
	dir := t.TempDir()
	projects, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if projects != nil {
		t.Errorf("expected nil projects, got %v", projects)
	}
}

func TestListProjects_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	projects, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestListProjects_SkipsFiles(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(filepath.Join(projDir, "real-project"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create a regular file in the projects directory (should be skipped).
	if err := os.WriteFile(filepath.Join(projDir, "not-a-project.txt"), []byte("skip"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	projects, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "real-project" {
		t.Errorf("Name = %q, want %q", projects[0].Name, "real-project")
	}
}

func TestListProjects_PathField(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(filepath.Join(projDir, "my-proj"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	projects, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	expectedPath := filepath.Join(projDir, "my-proj")
	if projects[0].Path != expectedPath {
		t.Errorf("Path = %q, want %q", projects[0].Path, expectedPath)
	}
}
