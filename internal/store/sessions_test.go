package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFindActiveSessions_MultipleActive(t *testing.T) {
	// Setup: create temp directory with test .jsonl files
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")

	// Create two project directories with sessions
	project1Dir := filepath.Join(projectsDir, "project1")
	project2Dir := filepath.Join(projectsDir, "project2")
	assert.NoError(t, os.MkdirAll(project1Dir, 0o755))
	assert.NoError(t, os.MkdirAll(project2Dir, 0o755))

	// Create active sessions (within threshold)
	now := time.Now()
	session1Path := filepath.Join(project1Dir, "session-1111-1111.jsonl")
	session2Path := filepath.Join(project2Dir, "session-2222-2222.jsonl")

	assert.NoError(t, os.WriteFile(session1Path, []byte("{}"), 0o644))
	assert.NoError(t, os.WriteFile(session2Path, []byte("{}"), 0o644))

	// Set modification times - session2 more recent than session1
	assert.NoError(t, os.Chtimes(session1Path, now.Add(-5*time.Minute), now.Add(-5*time.Minute)))
	assert.NoError(t, os.Chtimes(session2Path, now.Add(-2*time.Minute), now.Add(-2*time.Minute)))

	// Act: call FindActiveSessions with 15-minute threshold
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: check returned slice
	assert.NoError(t, err)
	assert.Len(t, sessions, 2)

	// Verify sort order (most recent first)
	assert.Equal(t, "session-2222-2222", sessions[0].SessionID)
	assert.Equal(t, "project2", sessions[0].ProjectName)
	assert.Equal(t, session2Path, sessions[0].Path)

	assert.Equal(t, "session-1111-1111", sessions[1].SessionID)
	assert.Equal(t, "project1", sessions[1].ProjectName)
	assert.Equal(t, session1Path, sessions[1].Path)
}

func TestFindActiveSessions_SingleActive(t *testing.T) {
	// Setup: create temp directory with single active session
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projectDir := filepath.Join(projectsDir, "myproject")
	assert.NoError(t, os.MkdirAll(projectDir, 0o755))

	now := time.Now()
	sessionPath := filepath.Join(projectDir, "session-abcd-1234.jsonl")
	assert.NoError(t, os.WriteFile(sessionPath, []byte("{}"), 0o644))
	assert.NoError(t, os.Chtimes(sessionPath, now.Add(-1*time.Minute), now.Add(-1*time.Minute)))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: returns single-element slice
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "session-abcd-1234", sessions[0].SessionID)
	assert.Equal(t, "myproject", sessions[0].ProjectName)
}

func TestFindActiveSessions_NoneActive(t *testing.T) {
	// Setup: create temp directory with old sessions (beyond threshold)
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projectDir := filepath.Join(projectsDir, "project1")
	assert.NoError(t, os.MkdirAll(projectDir, 0o755))

	now := time.Now()
	sessionPath := filepath.Join(projectDir, "session-old-1111.jsonl")
	assert.NoError(t, os.WriteFile(sessionPath, []byte("{}"), 0o644))
	// Set modification time to 20 minutes ago (beyond 15-minute threshold)
	assert.NoError(t, os.Chtimes(sessionPath, now.Add(-20*time.Minute), now.Add(-20*time.Minute)))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: returns nil (not an error)
	assert.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestFindActiveSessions_NoProjectsDir(t *testing.T) {
	// Setup: use temp directory without creating projects subdirectory
	tmpDir := t.TempDir()

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: returns nil (not an error)
	assert.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestFindActiveSessions_EmptyProjectsDir(t *testing.T) {
	// Setup: create empty projects directory
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	assert.NoError(t, os.MkdirAll(projectsDir, 0o755))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: returns nil (not an error)
	assert.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestFindActiveSessions_SortOrder(t *testing.T) {
	// Setup: create multiple sessions with different modification times
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projectDir := filepath.Join(projectsDir, "testproject")
	assert.NoError(t, os.MkdirAll(projectDir, 0o755))

	now := time.Now()

	// Create 4 sessions with different ages
	sessions := []struct {
		id     string
		ageMin int
	}{
		{"session-newest", 1},
		{"session-middle1", 5},
		{"session-middle2", 8},
		{"session-oldest", 12},
	}

	for _, s := range sessions {
		path := filepath.Join(projectDir, s.id+".jsonl")
		assert.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
		modTime := now.Add(-time.Duration(s.ageMin) * time.Minute)
		assert.NoError(t, os.Chtimes(path, modTime, modTime))
	}

	// Act
	result, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: verify most recent first
	assert.NoError(t, err)
	assert.Len(t, result, 4)

	// Should be in order: newest, middle1, middle2, oldest
	assert.Equal(t, "session-newest", result[0].SessionID)
	assert.Equal(t, "session-middle1", result[1].SessionID)
	assert.Equal(t, "session-middle2", result[2].SessionID)
	assert.Equal(t, "session-oldest", result[3].SessionID)

	// Verify timestamps are in descending order
	for i := 0; i < len(result)-1; i++ {
		assert.True(t, result[i].LastModified.After(result[i+1].LastModified),
			"Sessions should be sorted by LastModified descending")
	}
}

func TestFindActiveSessions_IgnoresNonJsonlFiles(t *testing.T) {
	// Setup: create directory with mix of .jsonl and other files
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projectDir := filepath.Join(projectsDir, "project1")
	assert.NoError(t, os.MkdirAll(projectDir, 0o755))

	now := time.Now()

	// Create .jsonl file (should be included)
	jsonlPath := filepath.Join(projectDir, "session-valid.jsonl")
	assert.NoError(t, os.WriteFile(jsonlPath, []byte("{}"), 0o644))
	assert.NoError(t, os.Chtimes(jsonlPath, now, now))

	// Create non-.jsonl files (should be ignored)
	txtPath := filepath.Join(projectDir, "notes.txt")
	assert.NoError(t, os.WriteFile(txtPath, []byte("notes"), 0o644))
	assert.NoError(t, os.Chtimes(txtPath, now, now))

	logPath := filepath.Join(projectDir, "session.log")
	assert.NoError(t, os.WriteFile(logPath, []byte("logs"), 0o644))
	assert.NoError(t, os.Chtimes(logPath, now, now))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: only .jsonl file is returned
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "session-valid", sessions[0].SessionID)
}

func TestFindActiveSessions_NestedProjectStructure(t *testing.T) {
	// Setup: create nested project structure
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")

	// Create nested projects (e.g., ~/.claude/projects/hash1/hash2/session.jsonl)
	nestedDir := filepath.Join(projectsDir, "outer-project", "inner-project")
	assert.NoError(t, os.MkdirAll(nestedDir, 0o755))

	now := time.Now()
	sessionPath := filepath.Join(nestedDir, "session-nested.jsonl")
	assert.NoError(t, os.WriteFile(sessionPath, []byte("{}"), 0o644))
	assert.NoError(t, os.Chtimes(sessionPath, now, now))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: finds nested session and extracts correct project name
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "session-nested", sessions[0].SessionID)
	// Project name should be the immediate parent directory
	assert.Equal(t, "inner-project", sessions[0].ProjectName)
}

func TestFindActiveSessions_SkipsSubagentFiles(t *testing.T) {
	// Setup: create session with subagent files (SAW pattern)
	tmpDir := t.TempDir()
	projectsDir := filepath.Join(tmpDir, "projects")
	projectDir := filepath.Join(projectsDir, "myproject")
	sessionDir := filepath.Join(projectDir, "session-main-1234")
	subagentsDir := filepath.Join(sessionDir, "subagents")
	assert.NoError(t, os.MkdirAll(subagentsDir, 0o755))

	now := time.Now()

	// Create main session file (should be included)
	mainSessionPath := filepath.Join(projectDir, "session-main-1234.jsonl")
	assert.NoError(t, os.WriteFile(mainSessionPath, []byte("{}"), 0o644))
	assert.NoError(t, os.Chtimes(mainSessionPath, now, now))

	// Create subagent files (should be excluded)
	subagent1Path := filepath.Join(subagentsDir, "agent-a123456789abc.jsonl")
	subagent2Path := filepath.Join(subagentsDir, "agent-b987654321def.jsonl")
	assert.NoError(t, os.WriteFile(subagent1Path, []byte("{}"), 0o644))
	assert.NoError(t, os.WriteFile(subagent2Path, []byte("{}"), 0o644))
	assert.NoError(t, os.Chtimes(subagent1Path, now, now))
	assert.NoError(t, os.Chtimes(subagent2Path, now, now))

	// Act
	sessions, err := FindActiveSessions(tmpDir, 15*time.Minute)

	// Assert: only main session returned, subagent files filtered out
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "session-main-1234", sessions[0].SessionID)
	assert.Equal(t, "myproject", sessions[0].ProjectName)
	assert.Equal(t, mainSessionPath, sessions[0].Path)
}
