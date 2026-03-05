package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
)

// helper: write a session stub under <tmpDir>.
// Creates a minimal JSONL transcript in projects/<id>/<id>.jsonl so that
// ParseAllSessionMeta (JSONL-primary) discovers the session, then writes a
// cache JSON in usage-data/session-meta/<id>.json with the desired field
// values. The JSONL mtime is backdated so the cache is always fresh.
func writeSessionMeta(t *testing.T, dir, id, startTime, projectPath string, inputTokens, outputTokens int) {
	t.Helper()

	// Write minimal JSONL stub so ParseAllSessionMeta discovers the session.
	projDir := filepath.Join(dir, "projects", id)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects/%s: %v", id, err)
	}
	jsonlPath := filepath.Join(projDir, id+".jsonl")
	stub := fmt.Sprintf("{\"type\":\"user\",\"sessionId\":%q,\"cwd\":%q,\"timestamp\":%q,\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"test\"}]}}\n", id, projectPath, startTime)
	if err := os.WriteFile(jsonlPath, []byte(stub), 0644); err != nil {
		t.Fatalf("write jsonl stub: %v", err)
	}
	// Backdate JSONL mtime so cache is always treated as fresh.
	past := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(jsonlPath, past, past)

	// Write cache JSON with desired field values.
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

// writeSessionMetaWithModels writes a session meta cache JSON that includes
// a model_usage map. This causes EstimateSessionCost to use the per-model
// pricing path (estimateFromModelUsage) instead of the single-tier fallback.
func writeSessionMetaWithModels(t *testing.T, dir, id, startTime, projectPath string, modelUsage map[string][2]int) {
	t.Helper()

	// Write minimal JSONL stub so ParseAllSessionMeta discovers the session.
	projDir := filepath.Join(dir, "projects", id)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects/%s: %v", id, err)
	}
	jsonlPath := filepath.Join(projDir, id+".jsonl")
	stub := fmt.Sprintf("{\"type\":\"user\",\"sessionId\":%q,\"cwd\":%q,\"timestamp\":%q,\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"test\"}]}}\n", id, projectPath, startTime)
	if err := os.WriteFile(jsonlPath, []byte(stub), 0644); err != nil {
		t.Fatalf("write jsonl stub: %v", err)
	}
	past := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(jsonlPath, past, past)

	// Build model_usage JSON.
	modelUsageJSON := "{"
	first := true
	totalInput, totalOutput := 0, 0
	for model, tokens := range modelUsage {
		if !first {
			modelUsageJSON += ","
		}
		modelUsageJSON += fmt.Sprintf("%q:{\"input_tokens\":%d,\"output_tokens\":%d}", model, tokens[0], tokens[1])
		totalInput += tokens[0]
		totalOutput += tokens[1]
		first = false
	}
	modelUsageJSON += "}"

	// Write cache JSON with model_usage field.
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
		"output_tokens": %d,
		"model_usage": %s
	}`, id, projectPath, startTime, totalInput, totalOutput, modelUsageJSON)
	if err := os.WriteFile(filepath.Join(metaDir, id+".json"), []byte(data), 0644); err != nil {
		t.Fatalf("write session meta: %v", err)
	}
}

// TestHandleGetSessionStats_PerModelCost verifies that when a session has
// ModelUsage populated with an Opus-tier model, the estimated cost reflects
// Opus pricing (which is 5x more expensive than Sonnet for input).
func TestHandleGetSessionStats_PerModelCost(t *testing.T) {
	dir := t.TempDir()

	// Session with Opus model usage: 1M input, 100K output.
	writeSessionMetaWithModels(t, dir, "opus-sess", "2026-01-15T10:00:00Z", "/home/user/myproject",
		map[string][2]int{
			"claude-3-opus-20240229": {1_000_000, 100_000},
		})

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	// Compute the expected Opus cost manually:
	// Input:  1_000_000 / 1M * $15.00 = $15.00
	// Output: 100_000 / 1M * $75.00  = $7.50
	// Total: $22.50
	opusPricing := analyzer.DefaultPricing["opus"]
	expectedCost := float64(1_000_000)/1_000_000.0*opusPricing.InputPerMillion +
		float64(100_000)/1_000_000.0*opusPricing.OutputPerMillion

	// Also compute what Sonnet would have been:
	// Input:  1_000_000 / 1M * $3.00 = $3.00
	// Output: 100_000 / 1M * $15.00 = $1.50
	// Total: $4.50
	sonnetPricing := analyzer.DefaultPricing["sonnet"]
	sonnetCost := float64(1_000_000)/1_000_000.0*sonnetPricing.InputPerMillion +
		float64(100_000)/1_000_000.0*sonnetPricing.OutputPerMillion

	// The per-model cost must be significantly higher than Sonnet pricing.
	if r.EstimatedCost <= sonnetCost {
		t.Errorf("EstimatedCost = %f, want > %f (Sonnet cost); per-model Opus pricing not applied",
			r.EstimatedCost, sonnetCost)
	}

	// Allow small floating-point tolerance.
	epsilon := 0.01
	if r.EstimatedCost < expectedCost-epsilon || r.EstimatedCost > expectedCost+epsilon {
		t.Errorf("EstimatedCost = %f, want ~%f (Opus pricing)", r.EstimatedCost, expectedCost)
	}
}

// TestHandleGetRecentSessions_PerModelCost verifies that the recent sessions
// list shows correct per-model costs: an Opus session costs more than a Sonnet
// session with identical token counts.
func TestHandleGetRecentSessions_PerModelCost(t *testing.T) {
	dir := t.TempDir()

	// Two sessions: one Opus, one Sonnet, same token counts.
	writeSessionMetaWithModels(t, dir, "opus-recent", "2026-01-15T10:00:00Z", "/home/user/proj",
		map[string][2]int{
			"claude-3-opus-20240229": {500_000, 50_000},
		})
	writeSessionMetaWithModels(t, dir, "sonnet-recent", "2026-01-14T10:00:00Z", "/home/user/proj",
		map[string][2]int{
			"claude-3-5-sonnet-20241022": {500_000, 50_000},
		})

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_recent_sessions", json.RawMessage(`{"n":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(RecentSessionsResult)
	if !ok {
		t.Fatalf("expected RecentSessionsResult, got %T", result)
	}

	if len(r.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(r.Sessions))
	}

	// Find each session's cost.
	var opusCost, sonnetCost float64
	for _, sess := range r.Sessions {
		switch sess.SessionID {
		case "opus-recent":
			opusCost = sess.EstimatedCost
		case "sonnet-recent":
			sonnetCost = sess.EstimatedCost
		}
	}

	if opusCost <= 0 {
		t.Fatalf("Opus session cost = %f, want > 0", opusCost)
	}
	if sonnetCost <= 0 {
		t.Fatalf("Sonnet session cost = %f, want > 0", sonnetCost)
	}

	// Opus pricing is 5x input and 5x output compared to Sonnet.
	// With same token counts, Opus cost should be ~5x Sonnet cost.
	if opusCost <= sonnetCost {
		t.Errorf("Opus cost (%f) <= Sonnet cost (%f); per-model pricing not applied",
			opusCost, sonnetCost)
	}

	// Verify the ratio is approximately 5x (within tolerance for output price ratio).
	ratio := opusCost / sonnetCost
	if ratio < 4.5 || ratio > 5.5 {
		t.Errorf("Opus/Sonnet cost ratio = %f, want ~5.0", ratio)
	}
}

// TestHandleGetSessionStats_MixedModels verifies that a session with both
// Opus and Haiku model usage produces a cost that is the weighted sum of
// per-model costs.
func TestHandleGetSessionStats_MixedModels(t *testing.T) {
	dir := t.TempDir()

	writeSessionMetaWithModels(t, dir, "mixed-sess", "2026-01-15T10:00:00Z", "/home/user/myproject",
		map[string][2]int{
			"claude-3-opus-20240229":  {500_000, 50_000},
			"claude-3-haiku-20240307": {500_000, 50_000},
		})

	s := newTestServer(dir, 0)
	result, err := callTool(s, "get_session_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionStatsResult)
	if !ok {
		t.Fatalf("expected SessionStatsResult, got %T", result)
	}

	// Expected cost: Opus portion + Haiku portion.
	opusPricing := analyzer.DefaultPricing["opus"]
	haikuPricing := analyzer.DefaultPricing["haiku"]
	expectedOpus := float64(500_000)/1_000_000.0*opusPricing.InputPerMillion +
		float64(50_000)/1_000_000.0*opusPricing.OutputPerMillion
	expectedHaiku := float64(500_000)/1_000_000.0*haikuPricing.InputPerMillion +
		float64(50_000)/1_000_000.0*haikuPricing.OutputPerMillion
	expectedTotal := expectedOpus + expectedHaiku

	epsilon := 0.01
	if r.EstimatedCost < expectedTotal-epsilon || r.EstimatedCost > expectedTotal+epsilon {
		t.Errorf("EstimatedCost = %f, want ~%f (weighted sum of Opus + Haiku)", r.EstimatedCost, expectedTotal)
	}

	// Verify it's NOT using single-tier Sonnet pricing on the total tokens.
	sonnetPricing := analyzer.DefaultPricing["sonnet"]
	sonnetCost := float64(1_000_000)/1_000_000.0*sonnetPricing.InputPerMillion +
		float64(100_000)/1_000_000.0*sonnetPricing.OutputPerMillion
	if r.EstimatedCost <= sonnetCost {
		t.Errorf("EstimatedCost (%f) <= Sonnet single-tier cost (%f); per-model pricing not applied",
			r.EstimatedCost, sonnetCost)
	}
}
