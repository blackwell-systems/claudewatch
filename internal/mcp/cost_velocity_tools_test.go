package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestCostVelocity_MCP_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addCostVelocityTools(s)

	_, err := callTool(s, "get_cost_velocity", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
	if err.Error() != "no active session found" {
		t.Errorf("expected 'no active session found', got %q", err.Error())
	}
}

func TestCostVelocity_MCP_ActiveSession(t *testing.T) {
	dir := t.TempDir()

	recentTS := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	// Build an active session JSONL with token usage in the window.
	// 500_000 input, 100_000 output
	// Cost = (500_000/1M)*3 + (100_000/1M)*15 = 1.5 + 1.5 = 3.0
	// CostPerMinute = 3.0/10 = 0.30 -> burning
	assistantLine := fmt.Sprintf(
		`{"type":"assistant","sessionId":"cost-sess-001","timestamp":%q,"message":{"role":"assistant","content":[],"usage":{"input_tokens":500000,"output_tokens":100000}}}`,
		recentTS,
	)
	lines := []string{
		`{"type":"user","sessionId":"cost-sess-001","timestamp":"2026-03-01T10:00:00Z"}`,
		assistantLine,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "cost-sess-001", lines)

	s := newTestServer(dir, 0)
	addCostVelocityTools(s)

	result, err := callTool(s, "get_cost_velocity", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostVelocityResult)
	if !ok {
		t.Fatalf("expected CostVelocityResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "cost-sess-001" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "cost-sess-001")
	}
	if r.WindowMinutes != 10 {
		t.Errorf("WindowMinutes = %f, want 10", r.WindowMinutes)
	}
	if r.WindowCostUSD < 2.9 || r.WindowCostUSD > 3.1 {
		t.Errorf("WindowCostUSD = %f, want ~3.0", r.WindowCostUSD)
	}
	if r.CostPerMinute < 0.29 || r.CostPerMinute > 0.31 {
		t.Errorf("CostPerMinute = %f, want ~0.30", r.CostPerMinute)
	}

	validStatuses := map[string]bool{"efficient": true, "normal": true, "burning": true}
	if !validStatuses[r.Status] {
		t.Errorf("Status = %q, want one of efficient/normal/burning", r.Status)
	}
	if r.Status != "burning" {
		t.Errorf("Status = %q, want %q for CostPerMinute=%f", r.Status, "burning", r.CostPerMinute)
	}
}

func TestCostVelocity_MCP_EfficientSession(t *testing.T) {
	dir := t.TempDir()

	recentTS := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	// 10_000 input, 2_000 output
	// Cost = (10_000/1M)*3 + (2_000/1M)*15 = 0.03 + 0.03 = 0.06
	// CostPerMinute = 0.06/10 = 0.006 -> efficient
	assistantLine := fmt.Sprintf(
		`{"type":"assistant","sessionId":"eff-cost-sess","timestamp":%q,"message":{"role":"assistant","content":[],"usage":{"input_tokens":10000,"output_tokens":2000}}}`,
		recentTS,
	)
	lines := []string{
		`{"type":"user","sessionId":"eff-cost-sess","timestamp":"2026-03-01T10:00:00Z"}`,
		assistantLine,
	}
	writeActiveToolJSONL(t, dir, "proj-hash", "eff-cost-sess", lines)

	s := newTestServer(dir, 0)
	addCostVelocityTools(s)

	result, err := callTool(s, "get_cost_velocity", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := result.(CostVelocityResult)
	if r.Status != "efficient" {
		t.Errorf("Status = %q, want %q", r.Status, "efficient")
	}
}
