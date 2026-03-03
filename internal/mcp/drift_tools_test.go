package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestGetDriftSignal_NoActiveSession verifies that an error is returned
// when no active JSONL session file exists.
func TestGetDriftSignal_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addDriftTools(s)

	_, err := callTool(s, "get_drift_signal", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
	if err.Error() != "no active session found" {
		t.Errorf("expected 'no active session found', got %q", err.Error())
	}
}

// TestGetDriftSignal_Exploring verifies that a session with only read-type
// tool calls (no Edit/Write anywhere) returns status "exploring".
func TestGetDriftSignal_Exploring(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addDriftTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"ok"}]}}`,
		`{"type":"assistant","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Grep","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","content":"ok"}]}}`,
		`{"type":"assistant","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:04Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-3","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-explore","timestamp":"2026-03-01T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-3","content":"ok"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-explore", lines)

	result, err := callTool(s, "get_drift_signal", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(DriftSignalResult)
	if !ok {
		t.Fatalf("expected DriftSignalResult, got %T", result)
	}

	if r.Status != "exploring" {
		t.Errorf("Status = %q, want %q", r.Status, "exploring")
	}
	if r.HasAnyEdit {
		t.Error("HasAnyEdit = true, want false")
	}
	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "sess-explore" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-explore")
	}
}

// TestGetDriftSignal_Implementing verifies that a session with at least one
// Edit tool_use call in the window returns status "implementing".
func TestGetDriftSignal_Implementing(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addDriftTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"ok"}]}}`,
		`{"type":"assistant","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","content":"ok"}]}}`,
		`{"type":"assistant","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:04Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-3","name":"Write","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-impl","timestamp":"2026-03-01T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-3","content":"ok"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-impl", lines)

	result, err := callTool(s, "get_drift_signal", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(DriftSignalResult)
	if !ok {
		t.Fatalf("expected DriftSignalResult, got %T", result)
	}

	if r.Status != "implementing" {
		t.Errorf("Status = %q, want %q", r.Status, "implementing")
	}
	if !r.HasAnyEdit {
		t.Error("HasAnyEdit = false, want true")
	}
	if r.WriteCalls < 1 {
		t.Errorf("WriteCalls = %d, want >= 1", r.WriteCalls)
	}
	if !r.Live {
		t.Error("Live = false, want true")
	}
}

// TestGetDriftSignal_Drifting verifies that a session where Edit was called
// earlier but the last 20 tool calls are all reads returns status "drifting".
func TestGetDriftSignal_Drifting(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addDriftTools(s)

	// Build session: 1 Edit call early, then 20 Read calls so the window
	// of 20 has 0 writes and 20/20 = 100% reads (well above 60% threshold).
	var lines []string
	tuIdx := 1

	// Early Edit call (outside the window of 20).
	lines = append(lines,
		fmt.Sprintf(`{"type":"assistant","sessionId":"sess-drift","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-%d","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`, tuIdx),
		fmt.Sprintf(`{"type":"user","sessionId":"sess-drift","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-%d","content":"ok"}]}}`, tuIdx),
	)
	tuIdx++

	// 20 Read calls to fill the window completely with reads.
	for i := 0; i < 20; i++ {
		ts1 := fmt.Sprintf("2026-03-01T10:%02d:00Z", i+1)
		ts2 := fmt.Sprintf("2026-03-01T10:%02d:01Z", i+1)
		lines = append(lines,
			fmt.Sprintf(`{"type":"assistant","sessionId":"sess-drift","timestamp":%q,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-%d","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`, ts1, tuIdx),
			fmt.Sprintf(`{"type":"user","sessionId":"sess-drift","timestamp":%q,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-%d","content":"ok"}]}}`, ts2, tuIdx),
		)
		tuIdx++
	}

	writeActiveToolJSONL(t, dir, "proj-hash", "sess-drift", lines)

	result, err := callTool(s, "get_drift_signal", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(DriftSignalResult)
	if !ok {
		t.Fatalf("expected DriftSignalResult, got %T", result)
	}

	if r.Status != "drifting" {
		t.Errorf("Status = %q, want %q", r.Status, "drifting")
	}
	if !r.HasAnyEdit {
		t.Error("HasAnyEdit = false, want true (Edit was called earlier in session)")
	}
	if r.WriteCalls != 0 {
		t.Errorf("WriteCalls = %d, want 0 (window should have no writes)", r.WriteCalls)
	}
	if r.ReadCalls < 9 {
		t.Errorf("ReadCalls = %d, want >= 9", r.ReadCalls)
	}
	if !r.Live {
		t.Error("Live = false, want true")
	}
}

// TestGetDriftSignal_MCP verifies that addDriftTools registers the
// get_drift_signal tool on the server.
func TestGetDriftSignal_MCP(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addDriftTools(s)

	found := false
	for _, tool := range s.tools {
		if tool.Name == "get_drift_signal" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("get_drift_signal tool not registered")
	}
}
