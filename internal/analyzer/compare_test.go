package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// testPricingCompare is a simple pricing config for tests.
var testPricingCompare = ModelPricing{
	InputPerMillion:      3.0,
	OutputPerMillion:     15.0,
	CacheReadPerMillion:  0.3,
	CacheWritePerMillion: 3.75,
}

// makeMeta creates a minimal SessionMeta for testing.
func makeMeta(id, startTime string, inputTokens, outputTokens, commits, toolErrors int) claude.SessionMeta {
	return claude.SessionMeta{
		SessionID:    id,
		StartTime:    startTime,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		GitCommits:   commits,
		ToolErrors:   toolErrors,
	}
}

// makeFacet creates a SessionFacet with the given friction counts.
func makeFacet(sessionID string, frictionCounts map[string]int) claude.SessionFacet {
	return claude.SessionFacet{
		SessionID:      sessionID,
		FrictionCounts: frictionCounts,
	}
}

func TestCompareSAWVsSequential_AllSAW(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 5, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 500_000, 50_000, 2, 1),
	}
	facets := []claude.SessionFacet{
		makeFacet("s1", map[string]int{"retry": 2}),
		makeFacet("s2", map[string]int{"retry": 1, "wrong_approach": 1}),
	}

	sawSessionIDs := map[string]int{"s1": 2, "s2": 1, "s3": 3}
	sawAgentCounts := map[string]int{"s1": 4, "s2": 2, "s3": 6}

	report := CompareSAWVsSequential(
		"myproject",
		sessions,
		facets,
		sawSessionIDs,
		sawAgentCounts,
		testPricingCompare,
		NoCacheRatio(),
		true,
	)

	if report.Project != "myproject" {
		t.Errorf("Project = %q, want %q", report.Project, "myproject")
	}

	if report.SAW.Count != 3 {
		t.Errorf("SAW.Count = %d, want 3", report.SAW.Count)
	}
	if report.Sequential.Count != 0 {
		t.Errorf("Sequential.Count = %d, want 0", report.Sequential.Count)
	}
	if len(report.Sessions) != 3 {
		t.Errorf("len(Sessions) = %d, want 3", len(report.Sessions))
	}

	// Verify all sessions are SAW.
	for _, sc := range report.Sessions {
		if !sc.IsSAW {
			t.Errorf("session %s should be SAW", sc.SessionID)
		}
	}

	// Sessions should be sorted descending by start time.
	if len(report.Sessions) >= 2 {
		if report.Sessions[0].StartTime < report.Sessions[1].StartTime {
			t.Error("Sessions not sorted descending by start time")
		}
	}
}

func TestCompareSAWVsSequential_AllSequential(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 3, 2),
		makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 5, 0),
	}
	facets := []claude.SessionFacet{
		makeFacet("s1", map[string]int{"retry": 2}),
	}

	// No SAW sessions.
	sawSessionIDs := map[string]int{}
	sawAgentCounts := map[string]int{}

	report := CompareSAWVsSequential(
		"myproject",
		sessions,
		facets,
		sawSessionIDs,
		sawAgentCounts,
		testPricingCompare,
		NoCacheRatio(),
		false,
	)

	if report.SAW.Count != 0 {
		t.Errorf("SAW.Count = %d, want 0", report.SAW.Count)
	}
	if report.Sequential.Count != 2 {
		t.Errorf("Sequential.Count = %d, want 2", report.Sequential.Count)
	}
	// includeSessions=false: Sessions should be nil/empty.
	if len(report.Sessions) != 0 {
		t.Errorf("Sessions should be empty when includeSessions=false, got %d", len(report.Sessions))
	}

	// All sessions should be non-SAW (WaveCount=0, AgentCount=0).
	// Test via the group count.
	if report.Sequential.AvgCommits <= 0 {
		t.Error("Sequential.AvgCommits should be > 0")
	}
}

func TestCompareSAWVsSequential_Mixed(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("saw1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 4, 0),
		makeMeta("saw2", "2026-01-02T10:00:00Z", 1_500_000, 150_000, 6, 1),
		makeMeta("seq1", "2026-01-03T10:00:00Z", 800_000, 80_000, 2, 3),
		makeMeta("seq2", "2026-01-04T10:00:00Z", 600_000, 60_000, 1, 0),
	}
	facets := []claude.SessionFacet{
		makeFacet("saw1", map[string]int{"retry": 1}),
		makeFacet("seq1", map[string]int{"wrong_approach": 2, "retry": 1}),
	}

	sawSessionIDs := map[string]int{"saw1": 2, "saw2": 1}
	sawAgentCounts := map[string]int{"saw1": 4, "saw2": 2}

	report := CompareSAWVsSequential(
		"myproject",
		sessions,
		facets,
		sawSessionIDs,
		sawAgentCounts,
		testPricingCompare,
		NoCacheRatio(),
		true,
	)

	if report.SAW.Count != 2 {
		t.Errorf("SAW.Count = %d, want 2", report.SAW.Count)
	}
	if report.Sequential.Count != 2 {
		t.Errorf("Sequential.Count = %d, want 2", report.Sequential.Count)
	}
	if len(report.Sessions) != 4 {
		t.Errorf("len(Sessions) = %d, want 4", len(report.Sessions))
	}

	// Verify SAW sessions have wave/agent counts set.
	for _, sc := range report.Sessions {
		if sc.IsSAW {
			if sc.WaveCount == 0 {
				t.Errorf("SAW session %s has WaveCount=0", sc.SessionID)
			}
			if sc.AgentCount == 0 {
				t.Errorf("SAW session %s has AgentCount=0", sc.SessionID)
			}
		}
	}

	// CostPerCommit should be 0 only if no commits exist.
	if report.SAW.CostPerCommit == 0 && report.SAW.AvgCommits > 0 {
		t.Error("SAW.CostPerCommit should be non-zero when AvgCommits > 0")
	}
	if report.Sequential.CostPerCommit == 0 && report.Sequential.AvgCommits > 0 {
		t.Error("Sequential.CostPerCommit should be non-zero when AvgCommits > 0")
	}

	// Verify friction fallback: seq2 has no facet, so friction = ToolErrors = 0.
	var seq2 *SessionComparison
	for i := range report.Sessions {
		if report.Sessions[i].SessionID == "seq2" {
			seq2 = &report.Sessions[i]
		}
	}
	if seq2 == nil {
		t.Fatal("seq2 session not found in report")
	}
	if seq2.Friction != 0 {
		t.Errorf("seq2 friction = %d, want 0 (falls back to ToolErrors=0)", seq2.Friction)
	}

	// saw2 has no facet, so friction = ToolErrors = 1.
	var saw2 *SessionComparison
	for i := range report.Sessions {
		if report.Sessions[i].SessionID == "saw2" {
			saw2 = &report.Sessions[i]
		}
	}
	if saw2 == nil {
		t.Fatal("saw2 session not found in report")
	}
	if saw2.Friction != 1 {
		t.Errorf("saw2 friction = %d, want 1 (falls back to ToolErrors=1)", saw2.Friction)
	}
}

func TestCompareSAWVsSequential_ZeroCommits(t *testing.T) {
	// All sessions have 0 commits — CostPerCommit should be 0, not Inf.
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 0, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 500_000, 50_000, 0, 0),
	}

	report := CompareSAWVsSequential(
		"proj",
		sessions,
		nil,
		map[string]int{},
		map[string]int{},
		testPricingCompare,
		NoCacheRatio(),
		false,
	)

	if report.Sequential.CostPerCommit != 0 {
		t.Errorf("CostPerCommit = %f, want 0 when commits=0", report.Sequential.CostPerCommit)
	}
}

func TestCompareSAWVsSequential_EmptySessions(t *testing.T) {
	report := CompareSAWVsSequential(
		"proj",
		nil,
		nil,
		map[string]int{},
		map[string]int{},
		testPricingCompare,
		NoCacheRatio(),
		true,
	)

	if report.SAW.Count != 0 || report.Sequential.Count != 0 {
		t.Error("Expected zero counts for empty sessions")
	}
	if len(report.Sessions) != 0 {
		t.Error("Expected no sessions for empty input")
	}
}
