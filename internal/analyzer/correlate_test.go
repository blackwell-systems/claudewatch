package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// makeSessions creates n synthetic SessionMeta records.
func makeSessions(n int) []claude.SessionMeta {
	sessions := make([]claude.SessionMeta, n)
	for i := 0; i < n; i++ {
		sessions[i] = claude.SessionMeta{
			SessionID:       string(rune('A' + i)),
			ProjectPath:     "/tmp/proj-a",
			DurationMinutes: 10 + i,
			InputTokens:     1000 * (i + 1),
			ToolCounts:      map[string]int{"Bash": i + 1},
			GitCommits:      i % 3,
			ToolErrors:      i % 2,
		}
	}
	return sessions
}

// TestCorrelateFactors_AllFactors_BasicSmoke builds 15 synthetic sessions and
// verifies GroupComparisons is populated and has_claude_md delta has the
// correct sign.
func TestCorrelateFactors_AllFactors_BasicSmoke(t *testing.T) {
	n := 15
	sessions := make([]claude.SessionMeta, n)
	projectPath := make(map[string]string)

	// Create temp directories: first 7 sessions have CLAUDE.md (low friction),
	// last 8 sessions do not (high friction).
	facets := make([]claude.SessionFacet, n)
	for i := 0; i < n; i++ {
		sid := string(rune('A' + i))
		dir := t.TempDir()
		sessions[i] = claude.SessionMeta{
			SessionID:       sid,
			ProjectPath:     dir,
			DurationMinutes: 10,
			InputTokens:     1000,
			ToolCounts:      map[string]int{"Bash": 5},
			GitCommits:      1,
			ToolErrors:      0,
		}
		projectPath[sid] = dir

		if i < 7 {
			// With CLAUDE.md: low friction
			if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# instructions"), 0644); err != nil {
				t.Fatal(err)
			}
			facets[i] = claude.SessionFacet{
				SessionID:      sid,
				FrictionCounts: map[string]int{"retry": 1},
			}
		} else {
			// Without CLAUDE.md: high friction
			facets[i] = claude.SessionFacet{
				SessionID:      sid,
				FrictionCounts: map[string]int{"retry": 3, "error": 2},
			}
		}
	}

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      facets,
		SAWSessions: map[string]bool{},
		ProjectPath: projectPath,
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Project:     "",
		Outcome:     OutcomeFriction,
		Factor:      "",
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalSessions != 15 {
		t.Errorf("expected TotalSessions=15, got %d", report.TotalSessions)
	}
	if len(report.GroupComparisons) == 0 {
		t.Fatal("expected GroupComparisons to be populated")
	}
	if report.SingleGroupComparison != nil {
		t.Error("expected SingleGroupComparison to be nil in all-factors mode")
	}

	// Find has_claude_md comparison.
	var found *GroupComparison
	for i := range report.GroupComparisons {
		if report.GroupComparisons[i].Factor == FactorHasClaudeMD {
			found = &report.GroupComparisons[i]
			break
		}
	}
	if found == nil {
		t.Fatal("has_claude_md not found in GroupComparisons")
	}
	// With CLAUDE.md: avg=1.0; without: avg=5.0; delta should be negative (true < false).
	if found.Delta >= 0 {
		t.Errorf("expected has_claude_md delta < 0 (with CLAUDE.md has lower friction), got %.2f", found.Delta)
	}
}

// TestCorrelateFactors_SingleFactor_Boolean verifies single boolean factor mode.
func TestCorrelateFactors_SingleFactor_Boolean(t *testing.T) {
	n := 12
	sessions := make([]claude.SessionMeta, n)
	projectPath := make(map[string]string)

	for i := 0; i < n; i++ {
		sid := string(rune('A' + i))
		dir := t.TempDir()
		sessions[i] = claude.SessionMeta{
			SessionID:   sid,
			ProjectPath: dir,
			ToolErrors:  i % 3,
		}
		projectPath[sid] = dir
		if i%2 == 0 {
			if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: projectPath,
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     OutcomeFriction,
		Factor:      FactorHasClaudeMD,
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.SingleGroupComparison == nil {
		t.Error("expected SingleGroupComparison to be non-nil")
	}
	if report.GroupComparisons != nil {
		t.Error("expected GroupComparisons to be nil in single-factor mode")
	}
	if report.SinglePearson != nil {
		t.Error("expected SinglePearson to be nil for boolean factor")
	}
}

// TestCorrelateFactors_SingleFactor_Numeric verifies single numeric factor mode.
func TestCorrelateFactors_SingleFactor_Numeric(t *testing.T) {
	sessions := makeSessions(12)

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: map[string]string{},
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     OutcomeFriction,
		Factor:      FactorToolCallCount,
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.SinglePearson == nil {
		t.Error("expected SinglePearson to be non-nil")
	}
	if report.PearsonResults != nil {
		t.Error("expected PearsonResults to be nil in single-factor mode")
	}
	if report.SingleGroupComparison != nil {
		t.Error("expected SingleGroupComparison to be nil for numeric factor")
	}
}

// TestCorrelateFactors_LowConfidenceFlagged verifies low-confidence flag is set
// when groups have fewer than 10 sessions.
func TestCorrelateFactors_LowConfidenceFlagged(t *testing.T) {
	n := 8
	sessions := make([]claude.SessionMeta, n)
	projectPath := make(map[string]string)

	for i := 0; i < n; i++ {
		sid := string(rune('A' + i))
		dir := t.TempDir()
		sessions[i] = claude.SessionMeta{
			SessionID:   sid,
			ProjectPath: dir,
			ToolErrors:  i % 2,
		}
		projectPath[sid] = dir
		if i < 4 {
			if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: projectPath,
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     OutcomeFriction,
		Factor:      "",
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundLowConf := false
	for _, gc := range report.GroupComparisons {
		if gc.LowConfidence {
			foundLowConf = true
			break
		}
	}
	if !foundLowConf {
		t.Error("expected at least one GroupComparison with LowConfidence=true")
	}
}

// TestCorrelateFactors_ProjectFilter verifies project filtering works correctly.
func TestCorrelateFactors_ProjectFilter(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/code/proj-alpha", ToolErrors: 1},
		{SessionID: "s2", ProjectPath: "/code/proj-alpha", ToolErrors: 2},
		{SessionID: "s3", ProjectPath: "/code/proj-alpha", ToolErrors: 0},
		{SessionID: "s4", ProjectPath: "/code/proj-beta", ToolErrors: 5},
		{SessionID: "s5", ProjectPath: "/code/proj-beta", ToolErrors: 4},
		{SessionID: "s6", ProjectPath: "/code/proj-beta", ToolErrors: 6},
	}

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: map[string]string{},
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Project:     "proj-alpha",
		Outcome:     OutcomeToolErrors,
		Factor:      "",
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalSessions != 3 {
		t.Errorf("expected TotalSessions=3 for proj-alpha, got %d", report.TotalSessions)
	}
}

// TestCorrelateFactors_InsufficientData verifies an error is returned for < 3 sessions.
func TestCorrelateFactors_InsufficientData(t *testing.T) {
	sessions := makeSessions(2)

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: map[string]string{},
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     OutcomeFriction,
	}

	_, err := CorrelateFactors(input)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

// TestCorrelateFactors_UnknownOutcome verifies an error is returned for an
// unrecognized outcome field.
func TestCorrelateFactors_UnknownOutcome(t *testing.T) {
	sessions := makeSessions(5)

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: map[string]string{},
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     "invalid",
	}

	_, err := CorrelateFactors(input)
	if err == nil {
		t.Error("expected error for unknown outcome, got nil")
	}
}

// TestPearsonCorrelation_PerfectPositive verifies r ≈ 1.0 for perfectly correlated data.
func TestPearsonCorrelation_PerfectPositive(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{2, 4, 6, 8, 10}
	r := pearsonR(x, y)
	if r < 0.9999 {
		t.Errorf("expected r ≈ 1.0 for perfect positive correlation, got %.6f", r)
	}
}

// TestPearsonCorrelation_NoVariance verifies r = 0 when x has no variance (no panic).
func TestPearsonCorrelation_NoVariance(t *testing.T) {
	x := []float64{3, 3, 3}
	y := []float64{1, 2, 3}
	r := pearsonR(x, y)
	if r != 0 {
		t.Errorf("expected r=0 when x has no variance, got %.6f", r)
	}
}

// TestCorrelateFactors_ZeroCommitOutcome verifies the zero_commit outcome
// produces only 0.0 or 1.0 values.
func TestCorrelateFactors_ZeroCommitOutcome(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/p/a", GitCommits: 0, ToolErrors: 0},
		{SessionID: "s2", ProjectPath: "/p/a", GitCommits: 1, ToolErrors: 1},
		{SessionID: "s3", ProjectPath: "/p/a", GitCommits: 0, ToolErrors: 0},
		{SessionID: "s4", ProjectPath: "/p/a", GitCommits: 2, ToolErrors: 0},
		{SessionID: "s5", ProjectPath: "/p/a", GitCommits: 0, ToolErrors: 1},
	}

	input := CorrelateInput{
		Sessions:    sessions,
		Facets:      nil,
		SAWSessions: map[string]bool{},
		ProjectPath: map[string]string{},
		Pricing:     DefaultPricing["sonnet"],
		CacheRatio:  NoCacheRatio(),
		Outcome:     OutcomeZeroCommit,
		Factor:      FactorToolCallCount,
	}

	report, err := CorrelateFactors(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify via facetsByID-free outcomeValue that values are 0 or 1.
	facetsByID := buildFacetIndex(nil)
	for _, sess := range sessions {
		v := outcomeValue(sess, OutcomeZeroCommit, facetsByID, DefaultPricing["sonnet"], NoCacheRatio())
		if v != 0.0 && v != 1.0 {
			t.Errorf("expected zero_commit outcome to be 0.0 or 1.0, got %f for session %s", v, sess.SessionID)
		}
	}

	// Also verify the report ran without issues.
	if report.TotalSessions != 5 {
		t.Errorf("expected TotalSessions=5, got %d", report.TotalSessions)
	}
}
