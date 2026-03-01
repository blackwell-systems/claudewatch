package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTranscriptJSONL writes a JSONL transcript file under
// <tmpDir>/projects/<projectHash>/<sessionID>.jsonl with the given lines.
func writeTranscriptJSONL(t *testing.T, dir, projectHash, sessionID string, lines []string) {
	t.Helper()
	projDir := filepath.Join(dir, "projects", projectHash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects/%s: %v", projectHash, err)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	path := filepath.Join(projDir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

// sawTranscriptLines returns two JSONL lines representing a SAW-tagged Task span.
// The assistant line launches the task and the user line completes it.
func sawTranscriptLines(sessionID, toolUseID, wave, agent, launchedAt, completedAt string) []string {
	assistantLine := `{"type":"assistant","timestamp":"` + launchedAt + `","sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + toolUseID + `","name":"Task","input":{"subagent_type":"general-purpose","description":"[SAW:` + wave + `:agent-` + agent + `] implement thing","prompt":"do the work"}}]}}`
	userLine := `{"type":"user","timestamp":"` + completedAt + `","sessionId":"` + sessionID + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":"done","is_error":false}]}}`
	return []string{assistantLine, userLine}
}

// untaggedTranscriptLines returns two JSONL lines for a Task span without a SAW tag.
func untaggedTranscriptLines(sessionID, toolUseID, launchedAt, completedAt string) []string {
	assistantLine := `{"type":"assistant","timestamp":"` + launchedAt + `","sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + toolUseID + `","name":"Task","input":{"subagent_type":"general-purpose","description":"regular task without SAW tag","prompt":"do work"}}]}}`
	userLine := `{"type":"user","timestamp":"` + completedAt + `","sessionId":"` + sessionID + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":"done","is_error":false}]}}`
	return []string{assistantLine, userLine}
}

// TestGetSAWSessions_Empty verifies that no transcript files returns an empty sessions list.
func TestGetSAWSessions_Empty(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	result, err := callTool(s, "get_saw_sessions", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SAWSessionsResult)
	if !ok {
		t.Fatalf("expected SAWSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(r.Sessions))
	}
}

// TestGetSAWSessions_ReturnsSAWOnly verifies that only SAW-tagged sessions are returned,
// not untagged sessions.
func TestGetSAWSessions_ReturnsSAWOnly(t *testing.T) {
	dir := t.TempDir()

	// Write a SAW-tagged session.
	sawLines := sawTranscriptLines("saw-session-1", "tu1", "wave1", "A", "2026-01-15T10:00:00Z", "2026-01-15T10:05:00Z")
	writeTranscriptJSONL(t, dir, "proj-hash-1", "saw-session-1", sawLines)

	// Write an untagged session.
	untaggedLines := untaggedTranscriptLines("plain-session-1", "tu2", "2026-01-15T09:00:00Z", "2026-01-15T09:10:00Z")
	writeTranscriptJSONL(t, dir, "proj-hash-2", "plain-session-1", untaggedLines)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_saw_sessions", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SAWSessionsResult)
	if !ok {
		t.Fatalf("expected SAWSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 1 {
		t.Fatalf("expected 1 SAW session, got %d", len(r.Sessions))
	}

	if r.Sessions[0].SessionID != "saw-session-1" {
		t.Errorf("SessionID = %q, want %q", r.Sessions[0].SessionID, "saw-session-1")
	}

	if r.Sessions[0].WaveCount != 1 {
		t.Errorf("WaveCount = %d, want 1", r.Sessions[0].WaveCount)
	}

	if r.Sessions[0].AgentCount != 1 {
		t.Errorf("AgentCount = %d, want 1", r.Sessions[0].AgentCount)
	}
}

// TestGetSAWSessions_LimitsN verifies that n=2 returns at most 2 sessions when 3 exist.
func TestGetSAWSessions_LimitsN(t *testing.T) {
	dir := t.TempDir()

	// Write 3 SAW sessions with different start times.
	for i, ts := range []string{"2026-01-10T10:00:00Z", "2026-01-11T10:00:00Z", "2026-01-12T10:00:00Z"} {
		sessionID := "saw-sess-" + string(rune('1'+i))
		toolUseID := "tu-" + string(rune('1'+i))
		projectHash := "proj-" + string(rune('1'+i))
		endTs := "2026-01-10T10:05:00Z"
		if i == 1 {
			endTs = "2026-01-11T10:05:00Z"
		} else if i == 2 {
			endTs = "2026-01-12T10:05:00Z"
		}
		lines := sawTranscriptLines(sessionID, toolUseID, "wave1", "A", ts, endTs)
		writeTranscriptJSONL(t, dir, projectHash, sessionID, lines)
	}

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_saw_sessions", json.RawMessage(`{"n":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SAWSessionsResult)
	if !ok {
		t.Fatalf("expected SAWSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 2 {
		t.Errorf("expected 2 sessions (n=2), got %d", len(r.Sessions))
	}

	// Should be sorted descending by start time — most recent first.
	if len(r.Sessions) >= 2 {
		if r.Sessions[0].StartTime < r.Sessions[1].StartTime {
			t.Errorf("sessions not sorted descending: first=%q second=%q", r.Sessions[0].StartTime, r.Sessions[1].StartTime)
		}
	}
}

// TestGetSAWWaveBreakdown_Found verifies wave breakdown for a session with 1 wave and 2 agents.
func TestGetSAWWaveBreakdown_Found(t *testing.T) {
	dir := t.TempDir()

	// Write a SAW session with 2 agents in wave1.
	agentALines := sawTranscriptLines("breakdown-sess", "tu-a", "wave1", "A", "2026-01-15T10:00:00Z", "2026-01-15T10:05:00Z")
	agentBLines := sawTranscriptLines("breakdown-sess", "tu-b", "wave1", "B", "2026-01-15T10:00:30Z", "2026-01-15T10:06:00Z")
	writeTranscriptJSONL(t, dir, "proj-bd", "breakdown-sess", append(agentALines, agentBLines...))

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_saw_wave_breakdown", json.RawMessage(`{"session_id":"breakdown-sess"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SAWWaveBreakdownResult)
	if !ok {
		t.Fatalf("expected SAWWaveBreakdownResult, got %T", result)
	}

	if r.SessionID != "breakdown-sess" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "breakdown-sess")
	}

	if len(r.Waves) != 1 {
		t.Fatalf("expected 1 wave, got %d", len(r.Waves))
	}

	wave := r.Waves[0]
	if wave.Wave != 1 {
		t.Errorf("Wave = %d, want 1", wave.Wave)
	}

	if wave.AgentCount != 2 {
		t.Errorf("AgentCount = %d, want 2", wave.AgentCount)
	}

	if wave.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", wave.DurationMs)
	}

	if wave.StartedAt == "" {
		t.Error("StartedAt is empty, want RFC3339 string")
	}

	if wave.EndedAt == "" {
		t.Error("EndedAt is empty, want RFC3339 string")
	}

	if len(wave.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(wave.Agents))
	}
}

// TestGetSAWWaveBreakdown_NotFound verifies that an unknown session_id returns an error.
func TestGetSAWWaveBreakdown_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	_, err := callTool(s, "get_saw_wave_breakdown", json.RawMessage(`{"session_id":"nonexistent-session"}`))
	if err == nil {
		t.Fatal("expected error for unknown session_id, got nil")
	}
}
