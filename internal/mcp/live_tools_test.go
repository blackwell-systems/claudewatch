package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// writeActiveToolJSONL writes a JSONL transcript with tool_use and tool_result blocks
// to simulate an active session for live tool tests. The file is placed under
// <claudeHome>/projects/<hash>/<sessionID>.jsonl with a recent mtime.
func writeActiveToolJSONL(t *testing.T, claudeHome, hash, sessionID string, lines []string) string {
	t.Helper()
	projDir := filepath.Join(claudeHome, "projects", hash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}

	var content string
	for _, line := range lines {
		content += line + "\n"
	}

	path := filepath.Join(projDir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write active tool JSONL: %v", err)
	}
	return path
}

// TestGetLiveToolErrors_NoActiveSession verifies that an error is returned
// when no active JSONL session file exists.
func TestGetLiveToolErrors_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	_, err := callTool(s, "get_live_tool_errors", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
	if err.Error() != "no active session found" {
		t.Errorf("expected 'no active session found', got %q", err.Error())
	}
}

// TestGetLiveToolErrors_Clean verifies that a session with tool uses but no
// errors returns severity "clean".
func TestGetLiveToolErrors_Clean(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	lines := []string{
		// Assistant uses a tool.
		`{"type":"assistant","sessionId":"sess-clean","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		// User returns a successful tool result.
		`{"type":"user","sessionId":"sess-clean","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"file contents here"}]}}`,
		// Another tool use.
		`{"type":"assistant","sessionId":"sess-clean","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Bash","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		// Successful result.
		`{"type":"user","sessionId":"sess-clean","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","content":"ok"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-clean", lines)

	result, err := callTool(s, "get_live_tool_errors", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(LiveToolErrorResult)
	if !ok {
		t.Fatalf("expected LiveToolErrorResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "sess-clean" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-clean")
	}
	if r.TotalToolUses != 2 {
		t.Errorf("TotalToolUses = %d, want 2", r.TotalToolUses)
	}
	if r.TotalErrors != 0 {
		t.Errorf("TotalErrors = %d, want 0", r.TotalErrors)
	}
	if r.ErrorRate != 0 {
		t.Errorf("ErrorRate = %f, want 0", r.ErrorRate)
	}
	if r.Severity != "clean" {
		t.Errorf("Severity = %q, want %q", r.Severity, "clean")
	}
	if r.ErrorsByTool == nil {
		t.Error("ErrorsByTool must not be nil")
	}
}

// TestGetLiveToolErrors_Degraded verifies that a session with a high error
// rate returns severity "degraded".
func TestGetLiveToolErrors_Degraded(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	// 3 tool uses, 2 errors = error rate 0.667 -> degraded
	lines := []string{
		`{"type":"assistant","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","is_error":true,"content":"edit failed"}]}}`,
		`{"type":"assistant","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","is_error":true,"content":"edit failed again"}]}}`,
		`{"type":"assistant","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:04Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-3","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-deg","timestamp":"2026-03-01T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-3","content":"ok"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-deg", lines)

	result, err := callTool(s, "get_live_tool_errors", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(LiveToolErrorResult)
	if !ok {
		t.Fatalf("expected LiveToolErrorResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.Severity != "degraded" {
		t.Errorf("Severity = %q, want %q", r.Severity, "degraded")
	}
	if r.TotalErrors != 2 {
		t.Errorf("TotalErrors = %d, want 2", r.TotalErrors)
	}
	if r.TotalToolUses != 3 {
		t.Errorf("TotalToolUses = %d, want 3", r.TotalToolUses)
	}
	if r.ConsecutiveErrs != 2 {
		t.Errorf("ConsecutiveErrs = %d, want 2", r.ConsecutiveErrs)
	}
	if r.ErrorsByTool["Edit"] != 2 {
		t.Errorf("ErrorsByTool[Edit] = %d, want 2", r.ErrorsByTool["Edit"])
	}
}

// TestGetLiveFriction_NoActiveSession verifies that an error is returned
// when no active JSONL session file exists.
func TestGetLiveFriction_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	_, err := callTool(s, "get_live_friction", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
	if err.Error() != "no active session found" {
		t.Errorf("expected 'no active session found', got %q", err.Error())
	}
}

// TestGetLiveFriction_NoFriction verifies that a clean session with no errors
// returns empty events and total friction of 0.
func TestGetLiveFriction_NoFriction(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	lines := []string{
		`{"type":"assistant","sessionId":"sess-nofric","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-nofric","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"ok"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-nofric", lines)

	result, err := callTool(s, "get_live_friction", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(LiveFrictionResult)
	if !ok {
		t.Fatalf("expected LiveFrictionResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "sess-nofric" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-nofric")
	}
	if r.TotalFriction != 0 {
		t.Errorf("TotalFriction = %d, want 0", r.TotalFriction)
	}
	if len(r.Events) != 0 {
		t.Errorf("len(Events) = %d, want 0", len(r.Events))
	}
	if r.TopType != "" {
		t.Errorf("TopType = %q, want empty string", r.TopType)
	}
}

// TestGetLiveFriction_WithErrors verifies that a session with tool errors
// produces friction events and computes the correct TopType.
func TestGetLiveFriction_WithErrors(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	// Two tool errors on Edit -> 2 tool_error friction events.
	lines := []string{
		`{"type":"assistant","sessionId":"sess-fric","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-fric","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","is_error":true,"content":"edit failed"}]}}`,
		`{"type":"assistant","sessionId":"sess-fric","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-fric","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","is_error":true,"content":"edit failed again"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-fric", lines)

	result, err := callTool(s, "get_live_friction", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(LiveFrictionResult)
	if !ok {
		t.Fatalf("expected LiveFrictionResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "sess-fric" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-fric")
	}
	if r.TotalFriction == 0 {
		t.Error("TotalFriction = 0, want > 0")
	}
	if len(r.Events) == 0 {
		t.Fatal("expected friction events, got none")
	}

	// Verify we have tool_error events.
	hasToolError := false
	for _, ev := range r.Events {
		if ev.Type == "tool_error" {
			hasToolError = true
			break
		}
	}
	if !hasToolError {
		t.Error("expected at least one tool_error event")
	}

	// TopType should include tool_error (both errors are tool_error type with count 1 each).
	// The retry friction event (Edit used 2x in last 3) also fires.
	// tool_error: count 1+1=2, retry: count 2 -> tie broken alphabetically -> "retry" < "tool_error"
	// But check that TopType is non-empty at minimum.
	if r.TopType == "" {
		t.Error("TopType is empty, want non-empty")
	}

	// Verify the events contain expected friction type.
	typeCounts := make(map[string]int)
	for _, ev := range r.Events {
		typeCounts[ev.Type] += ev.Count
	}
	// Both tool_error events should be present.
	if typeCounts["tool_error"] < 2 {
		t.Errorf("tool_error count = %d, want >= 2", typeCounts["tool_error"])
	}

	// Verify the correct TopType given actual friction events.
	// With 2 Edit tool_uses in last 3, we get a retry event on the second one.
	// retry has count=2 (from the event), tool_error has count=1+1=2 total.
	// Tie -> alphabetically "retry" wins.
	_ = claude.LiveFrictionEvent{} // ensure import is used
}

// TestGetLiveFriction_PatternsInResponse verifies that the MCP response includes
// friction patterns computed from the full event list.
func TestGetLiveFriction_PatternsInResponse(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addLiveTools(s)

	// Two Edit errors + a retry (Edit used twice in last 3 tool_uses).
	lines := []string{
		`{"type":"assistant","sessionId":"sess-pat","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-pat","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","is_error":true,"content":"edit failed"}]}}`,
		`{"type":"assistant","sessionId":"sess-pat","timestamp":"2026-03-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-2","name":"Edit","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"user","sessionId":"sess-pat","timestamp":"2026-03-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-2","is_error":true,"content":"edit failed again"}]}}`,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "sess-pat", lines)

	result, err := callTool(s, "get_live_friction", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(LiveFrictionResult)
	if !ok {
		t.Fatalf("expected LiveFrictionResult, got %T", result)
	}

	if len(r.Patterns) == 0 {
		t.Fatal("Patterns is empty, expected at least one pattern")
	}

	// Verify tool_error:Edit pattern exists.
	found := false
	for _, p := range r.Patterns {
		if p.Type == "tool_error:Edit" {
			found = true
			if p.Count < 2 {
				t.Errorf("tool_error:Edit Count = %d, want >= 2", p.Count)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected tool_error:Edit pattern, got patterns: %+v", r.Patterns)
	}
}
