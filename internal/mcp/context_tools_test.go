package mcp

import (
	"encoding/json"
	"testing"
)

// TestContextPressure_NoActiveSession verifies that an error is returned
// when no active JSONL session file exists.
func TestContextPressure_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addContextTools(s)

	_, err := callTool(s, "get_context_pressure", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
	if err.Error() != "no active session found" {
		t.Errorf("expected 'no active session found', got %q", err.Error())
	}
}

// TestContextPressure_Comfortable verifies that a session with low token usage
// returns status "comfortable".
func TestContextPressure_Comfortable(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addContextTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-ctx","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":10000,"output_tokens":2000}}}`,
		`{"type":"assistant","sessionId":"sess-ctx","timestamp":"2026-03-01T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"world"}],"usage":{"input_tokens":20000,"output_tokens":3000}}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-ctx", lines)

	result, err := callTool(s, "get_context_pressure", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ContextPressureResult)
	if !ok {
		t.Fatalf("expected ContextPressureResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "sess-ctx" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-ctx")
	}
	if r.TotalInputTokens != 30000 {
		t.Errorf("TotalInputTokens = %d, want 30000", r.TotalInputTokens)
	}
	if r.TotalOutputTokens != 5000 {
		t.Errorf("TotalOutputTokens = %d, want 5000", r.TotalOutputTokens)
	}
	if r.TotalTokens != 35000 {
		t.Errorf("TotalTokens = %d, want 35000", r.TotalTokens)
	}
	if r.Status != "comfortable" {
		t.Errorf("Status = %q, want %q", r.Status, "comfortable")
	}
	// EstimatedUsage = 20000 / 200000 = 0.1
	if r.EstimatedUsage != 0.1 {
		t.Errorf("EstimatedUsage = %f, want 0.1", r.EstimatedUsage)
	}
}

// TestContextPressure_WithCompactions verifies that compaction (summary) entries
// are counted and the result is correct.
func TestContextPressure_WithCompactions(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addContextTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-comp","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":80000,"output_tokens":5000}}}`,
		`{"type":"summary","sessionId":"sess-comp","timestamp":"2026-03-01T10:01:00Z"}`,
		`{"type":"assistant","sessionId":"sess-comp","timestamp":"2026-03-01T10:02:00Z","message":{"role":"assistant","content":[{"type":"text","text":"after compaction"}],"usage":{"input_tokens":40000,"output_tokens":3000}}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-comp", lines)

	result, err := callTool(s, "get_context_pressure", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ContextPressureResult)
	if !ok {
		t.Fatalf("expected ContextPressureResult, got %T", result)
	}

	if r.SessionID != "sess-comp" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-comp")
	}
	if r.Compactions != 1 {
		t.Errorf("Compactions = %d, want 1", r.Compactions)
	}
	if r.TotalInputTokens != 120000 {
		t.Errorf("TotalInputTokens = %d, want 120000", r.TotalInputTokens)
	}
	// Last input_tokens = 40000 -> 40000/200000 = 0.2 -> comfortable
	if r.Status != "comfortable" {
		t.Errorf("Status = %q, want %q", r.Status, "comfortable")
	}
}

// TestContextPressure_Critical verifies that high context usage returns "critical".
func TestContextPressure_Critical(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addContextTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-crit","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"lots of context"}],"usage":{"input_tokens":190000,"output_tokens":5000}}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-crit", lines)

	result, err := callTool(s, "get_context_pressure", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ContextPressureResult)
	if !ok {
		t.Fatalf("expected ContextPressureResult, got %T", result)
	}

	if r.Status != "critical" {
		t.Errorf("Status = %q, want %q", r.Status, "critical")
	}
	// 190000 / 200000 = 0.95
	if r.EstimatedUsage != 0.95 {
		t.Errorf("EstimatedUsage = %f, want 0.95", r.EstimatedUsage)
	}
}

// TestContextPressure_MCP verifies the tool is registered and callable.
func TestContextPressure_MCP(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addContextTools(s)

	// Verify tool is registered.
	found := false
	for _, tool := range s.tools {
		if tool.Name == "get_context_pressure" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("get_context_pressure tool not registered")
	}
}
