package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: write a session-meta JSON file under <tmpDir>/usage-data/session-meta/<id>.json
func writeSessionMeta(t *testing.T, dir, id, startTime, projectPath string, inputTokens, outputTokens int) {
	t.Helper()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir session-meta: %v", err)
	}
	data := fmt.Sprintf(`{
		"session_id": %q,
		"project_path": %q,
		"start_time": %q,
		"duration_minutes": 30,
		"input_tokens": %d,
		"output_tokens": %d
	}`, id, projectPath, startTime, inputTokens, outputTokens)
	if err := os.WriteFile(filepath.Join(metaDir, id+".json"), []byte(data), 0644); err != nil {
		t.Fatalf("write session meta: %v", err)
	}
}

// helper: write a facet JSON file under <tmpDir>/usage-data/facets/<id>.json
func writeFacet(t *testing.T, dir, id string, frictionCounts map[string]int) {
	t.Helper()
	facetDir := filepath.Join(dir, "usage-data", "facets")
	if err := os.MkdirAll(facetDir, 0755); err != nil {
		t.Fatalf("mkdir facets: %v", err)
	}
	countsJSON, _ := json.Marshal(frictionCounts)
	data := fmt.Sprintf(`{"session_id":%q,"friction_counts":%s}`, id, countsJSON)
	if err := os.WriteFile(filepath.Join(facetDir, id+".json"), []byte(data), 0644); err != nil {
		t.Fatalf("write facet: %v", err)
	}
}

// newTestServer creates a Server pointing at the given tmpDir with no budget.
func newTestServer(tmpDir string, budgetUSD float64) *Server {
	s := &Server{
		claudeHome:   tmpDir,
		budgetUSD:    budgetUSD,
		tagStorePath: filepath.Join(tmpDir, "session-tags.json"),
	}
	addTools(s)
	return s
}

// callTool invokes the named tool handler and returns the typed result.
func callTool(s *Server, name string, args json.RawMessage) (any, error) {
	for _, tool := range s.tools {
		if tool.Name == name {
			return tool.Handler(args)
		}
	}
	return nil, fmt.Errorf("tool not found: %s", name)
}

// TestGetSessionStats_NoSessions verifies that an error is returned when there are no sessions.
func TestGetSessionStats_NoSessions(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	_, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no sessions, got nil")
	}
}

// TestGetSessionStats_SingleSession verifies cost > 0 and correct project name.
func TestGetSessionStats_SingleSession(t *testing.T) {
	dir := t.TempDir()
	writeSessionMeta(t, dir, "sess-001", "2026-01-15T10:00:00Z", "/home/user/myproject", 1_000_000, 100_000)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	if r.EstimatedCost <= 0 {
		t.Errorf("EstimatedCost = %f, want > 0", r.EstimatedCost)
	}
	if r.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", r.ProjectName, "myproject")
	}
	if r.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-001")
	}
}

// TestGetCostBudget_NoSessions verifies zero spend and no over-budget when there are no sessions.
func TestGetCostBudget_NoSessions(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 10.0)

	result, err := callTool(s, "get_cost_budget", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostBudgetResult)
	if !ok {
		t.Fatalf("expected CostBudgetResult, got %T", result)
	}

	if r.TodaySpendUSD != 0 {
		t.Errorf("TodaySpendUSD = %f, want 0", r.TodaySpendUSD)
	}
	if r.OverBudget {
		t.Error("OverBudget = true, want false")
	}
}

// TestGetCostBudget_OverBudget verifies that sessions dated today with real tokens trigger over-budget.
func TestGetCostBudget_OverBudget(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Format("2006-01-02") + "T12:00:00Z"
	// Use very large token counts so that cost definitely exceeds a $0.01 budget.
	writeSessionMeta(t, dir, "today-sess", today, "/home/user/proj", 10_000_000, 1_000_000)

	s := newTestServer(dir, 0.01)

	result, err := callTool(s, "get_cost_budget", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostBudgetResult)
	if !ok {
		t.Fatalf("expected CostBudgetResult, got %T", result)
	}

	if !r.OverBudget {
		t.Errorf("OverBudget = false, want true (TodaySpendUSD=%f, budget=%f)", r.TodaySpendUSD, r.DailyBudgetUSD)
	}
	if r.TodaySpendUSD <= 0 {
		t.Errorf("TodaySpendUSD = %f, want > 0", r.TodaySpendUSD)
	}
}

// TestGetRecentSessions_DefaultN verifies that 5 sessions are returned by default when 10 exist.
func TestGetRecentSessions_DefaultN(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		startTime := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMeta(t, dir, id, startTime, "/home/user/proj", 1000, 500)
	}

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_recent_sessions", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(RecentSessionsResult)
	if !ok {
		t.Fatalf("expected RecentSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 5 {
		t.Errorf("len(Sessions) = %d, want 5", len(r.Sessions))
	}
}

// TestGetRecentSessions_CustomN verifies that passing n=3 returns 3 sessions.
func TestGetRecentSessions_CustomN(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		startTime := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMeta(t, dir, id, startTime, "/home/user/proj", 1000, 500)
	}

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_recent_sessions", json.RawMessage(`{"n":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(RecentSessionsResult)
	if !ok {
		t.Fatalf("expected RecentSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 3 {
		t.Errorf("len(Sessions) = %d, want 3", len(r.Sessions))
	}
}

// TestGetRecentSessions_FrictionScore verifies that friction counts are summed correctly.
func TestGetRecentSessions_FrictionScore(t *testing.T) {
	dir := t.TempDir()
	writeSessionMeta(t, dir, "friction-sess", "2026-01-15T10:00:00Z", "/home/user/proj", 1000, 500)
	writeFacet(t, dir, "friction-sess", map[string]int{
		"wrong_approach": 2,
		"off_track":      1,
	})

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_recent_sessions", json.RawMessage(`{"n":5}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(RecentSessionsResult)
	if !ok {
		t.Fatalf("expected RecentSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(r.Sessions))
	}

	if r.Sessions[0].FrictionScore != 3 {
		t.Errorf("FrictionScore = %d, want 3", r.Sessions[0].FrictionScore)
	}
}

// writeActiveJSONL writes a minimal JSONL transcript to
// <claudeHome>/projects/<hash>/<sessionID>.jsonl
// with a recent mtime (just-created), simulating an in-progress session.
func writeActiveJSONL(t *testing.T, claudeHome, hash, sessionID string, inputTokens, outputTokens int) string {
	t.Helper()
	projDir := filepath.Join(claudeHome, "projects", hash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}

	// Build a minimal 2-line JSONL: one user entry, one assistant entry with usage.
	userLine := fmt.Sprintf(`{"type":"user","sessionId":%q,"timestamp":"2026-03-01T10:00:00Z"}`, sessionID)
	assistantLine := fmt.Sprintf(
		`{"type":"assistant","sessionId":%q,"timestamp":"2026-03-01T10:01:00Z","message":{"usage":{"input_tokens":%d,"output_tokens":%d}}}`,
		sessionID, inputTokens, outputTokens,
	)
	content := userLine + "\n" + assistantLine + "\n"

	path := filepath.Join(projDir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write active JSONL: %v", err)
	}
	return path
}

// TestGetSessionStats_LiveSession_TakesPrecedence verifies that when both an
// active JSONL file and a closed session meta file exist, the live session is
// returned (Live: true).
func TestGetSessionStats_LiveSession_TakesPrecedence(t *testing.T) {
	dir := t.TempDir()

	// Write a closed session meta file (older).
	writeSessionMeta(t, dir, "closed-sess", "2026-01-01T08:00:00Z", "/home/user/oldproject", 500_000, 50_000)

	// Write an active JSONL file (recent mtime — created right now).
	writeActiveJSONL(t, dir, "proj-hash-abc", "live-sess-001", 1_000_000, 200_000)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	if !r.Live {
		t.Errorf("Live = false, want true (live session should take precedence)")
	}
	if r.SessionID != "live-sess-001" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "live-sess-001")
	}
}

// TestGetSessionStats_LiveSession_Fields verifies that SessionID, ProjectName,
// InputTokens, OutputTokens, EstimatedCost, and Live are correctly populated
// from the active JSONL.
func TestGetSessionStats_LiveSession_Fields(t *testing.T) {
	dir := t.TempDir()

	writeActiveJSONL(t, dir, "my-project-hash", "test-session-xyz", 800_000, 100_000)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "test-session-xyz" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "test-session-xyz")
	}
	if r.ProjectName != "my-project-hash" {
		t.Errorf("ProjectName = %q, want %q", r.ProjectName, "my-project-hash")
	}
	if r.InputTokens != 800_000 {
		t.Errorf("InputTokens = %d, want %d", r.InputTokens, 800_000)
	}
	if r.OutputTokens != 100_000 {
		t.Errorf("OutputTokens = %d, want %d", r.OutputTokens, 100_000)
	}
	if r.EstimatedCost <= 0 {
		t.Errorf("EstimatedCost = %f, want > 0", r.EstimatedCost)
	}
	if r.StartTime == "" {
		t.Error("StartTime is empty, want non-empty")
	}
}

// TestGetSessionStats_NoActiveFallsBackToClosed verifies that when no active
// session exists (empty projects dir), the closed session is returned with Live: false.
func TestGetSessionStats_NoActiveFallsBackToClosed(t *testing.T) {
	dir := t.TempDir()

	// Only a closed session meta file; no active JSONL.
	writeSessionMeta(t, dir, "closed-only", "2026-01-10T09:00:00Z", "/home/user/closedproject", 200_000, 30_000)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	if r.Live {
		t.Error("Live = true, want false (no active session present)")
	}
	if r.SessionID != "closed-only" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "closed-only")
	}
}

// TestGetSessionStats_LiveField_False_WhenClosed explicitly verifies that the
// Live field is false for the normal closed-session path.
func TestGetSessionStats_LiveField_False_WhenClosed(t *testing.T) {
	dir := t.TempDir()
	writeSessionMeta(t, dir, "sess-001", "2026-01-15T10:00:00Z", "/home/user/myproject", 1_000_000, 100_000)

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	if r.Live {
		t.Errorf("Live = true, want false for closed session")
	}
}
