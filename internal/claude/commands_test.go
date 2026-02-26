package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListCommands_ValidCommands(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create two command files.
	if err := os.WriteFile(filepath.Join(cmdDir, "review.md"), []byte("Review the code"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "deploy.md"), []byte("Deploy to production"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}

	found := map[string]string{}
	for _, c := range commands {
		found[c.Name] = c.Content
	}
	if found["review"] != "Review the code" {
		t.Errorf("review content = %q, want %q", found["review"], "Review the code")
	}
	if found["deploy"] != "Deploy to production" {
		t.Errorf("deploy content = %q, want %q", found["deploy"], "Deploy to production")
	}
}

func TestListCommands_MissingDir(t *testing.T) {
	dir := t.TempDir()
	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if commands != nil {
		t.Errorf("expected nil commands for missing dir, got %v", commands)
	}
}

func TestListCommands_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(commands))
	}
}

func TestListCommands_SkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a .md file and a .txt file.
	if err := os.WriteFile(filepath.Join(cmdDir, "valid.md"), []byte("valid command"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "invalid.txt"), []byte("not a command"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].Name != "valid" {
		t.Errorf("Name = %q, want %q", commands[0].Name, "valid")
	}
}

func TestListCommands_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(filepath.Join(cmdDir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(cmdDir, "top-level.md"), []byte("command"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].Name != "top-level" {
		t.Errorf("Name = %q, want %q", commands[0].Name, "top-level")
	}
}

func TestListCommands_PathField(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(cmdDir, "test-cmd.md"), []byte("content"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	commands, err := ListCommands(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	expectedPath := filepath.Join(cmdDir, "test-cmd.md")
	if commands[0].Path != expectedPath {
		t.Errorf("Path = %q, want %q", commands[0].Path, expectedPath)
	}
}
