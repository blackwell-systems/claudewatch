package analyzer

import (
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestAnalyzeFrictionPersistence_Empty(t *testing.T) {
	result := AnalyzeFrictionPersistence(nil, nil)
	if len(result.Patterns) != 0 {
		t.Errorf("expected 0 patterns for nil input, got %d", len(result.Patterns))
	}

	result = AnalyzeFrictionPersistence([]claude.SessionFacet{}, []claude.SessionMeta{})
	if len(result.Patterns) != 0 {
		t.Errorf("expected 0 patterns for empty input, got %d", len(result.Patterns))
	}
}

func TestAnalyzeFrictionPersistence_NoMatchingMetas(t *testing.T) {
	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 1}},
	}
	// No metas to match, so no timed facets.
	result := AnalyzeFrictionPersistence(facets, nil)
	if len(result.Patterns) != 0 {
		t.Errorf("expected 0 patterns when no metas match, got %d", len(result.Patterns))
	}
}

// makeTime creates a time from a date string for testing convenience.
func makeTime(t *testing.T, dateStr string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t.Fatalf("bad test date %q: %v", dateStr, err)
	}
	return ts
}

func TestAnalyzeFrictionPersistence_SingleSession(t *testing.T) {
	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 2}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result.Patterns))
	}

	p := result.Patterns[0]
	if p.FrictionType != "wrong_approach" {
		t.Errorf("expected friction type 'wrong_approach', got %q", p.FrictionType)
	}
	if p.OccurrenceCount != 1 {
		t.Errorf("expected occurrence count 1, got %d", p.OccurrenceCount)
	}
	if p.TotalSessions != 1 {
		t.Errorf("expected total sessions 1, got %d", p.TotalSessions)
	}
	if p.Frequency != 1.0 {
		t.Errorf("expected frequency 1.0, got %f", p.Frequency)
	}
	if p.WeeklyTrend != "stable" {
		t.Errorf("expected trend 'stable' for single session, got %q", p.WeeklyTrend)
	}
	if p.ConsecutiveWeeks != 1 {
		t.Errorf("expected 1 consecutive week, got %d", p.ConsecutiveWeeks)
	}
	if p.Stale {
		t.Error("expected stale=false for single week")
	}
}

func TestAnalyzeFrictionPersistence_ImprovingTrend(t *testing.T) {
	// 4 weeks: weeks 1-2 have friction in many sessions, weeks 3-4 in fewer.
	facets := []claude.SessionFacet{
		// Week 1 (Mon Jan 5 2026): 3 sessions with friction
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s3", FrictionCounts: map[string]int{"wrong_approach": 1}},
		// Week 2 (Mon Jan 12): 3 sessions with friction
		{SessionID: "s4", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s5", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s6", FrictionCounts: map[string]int{"wrong_approach": 1}},
		// Week 3 (Mon Jan 19): 1 session with friction
		{SessionID: "s7", FrictionCounts: map[string]int{"wrong_approach": 1}},
		// Week 4 (Mon Jan 26): 1 session with friction
		{SessionID: "s8", FrictionCounts: map[string]int{"wrong_approach": 1}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-06T10:00:00Z"},
		{SessionID: "s3", StartTime: "2026-01-07T10:00:00Z"},
		{SessionID: "s4", StartTime: "2026-01-12T10:00:00Z"},
		{SessionID: "s5", StartTime: "2026-01-13T10:00:00Z"},
		{SessionID: "s6", StartTime: "2026-01-14T10:00:00Z"},
		{SessionID: "s7", StartTime: "2026-01-19T10:00:00Z"},
		{SessionID: "s8", StartTime: "2026-01-26T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result.Patterns))
	}

	p := result.Patterns[0]
	if p.WeeklyTrend != "improving" {
		t.Errorf("expected trend 'improving', got %q", p.WeeklyTrend)
	}
	if result.ImprovingCount != 1 {
		t.Errorf("expected improving count 1, got %d", result.ImprovingCount)
	}
}

func TestAnalyzeFrictionPersistence_WorseningTrend(t *testing.T) {
	// Opposite: weeks 1-2 have few, weeks 3-4 have many.
	facets := []claude.SessionFacet{
		// Week 1: 1 session
		{SessionID: "s1", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		// Week 2: 1 session
		{SessionID: "s2", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		// Week 3: 3 sessions
		{SessionID: "s3", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		{SessionID: "s4", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		{SessionID: "s5", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		// Week 4: 3 sessions
		{SessionID: "s6", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		{SessionID: "s7", FrictionCounts: map[string]int{"misunderstood_request": 1}},
		{SessionID: "s8", FrictionCounts: map[string]int{"misunderstood_request": 1}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-12T10:00:00Z"},
		{SessionID: "s3", StartTime: "2026-01-19T10:00:00Z"},
		{SessionID: "s4", StartTime: "2026-01-20T10:00:00Z"},
		{SessionID: "s5", StartTime: "2026-01-21T10:00:00Z"},
		{SessionID: "s6", StartTime: "2026-01-26T10:00:00Z"},
		{SessionID: "s7", StartTime: "2026-01-27T10:00:00Z"},
		{SessionID: "s8", StartTime: "2026-01-28T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result.Patterns))
	}

	p := result.Patterns[0]
	if p.WeeklyTrend != "worsening" {
		t.Errorf("expected trend 'worsening', got %q", p.WeeklyTrend)
	}
	// 4 consecutive worsening weeks triggers stale, which takes priority
	// over WorseningCount in the switch. Verify stale instead.
	if !p.Stale {
		t.Errorf("expected pattern to be stale (4 consecutive worsening weeks)")
	}
	if result.StaleCount != 1 {
		t.Errorf("expected stale count 1, got %d", result.StaleCount)
	}
}

func TestAnalyzeFrictionPersistence_StalePattern(t *testing.T) {
	// Same friction count every week for 4 weeks -> stable + 4 consecutive -> stale.
	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s3", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s4", FrictionCounts: map[string]int{"wrong_approach": 1}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-12T10:00:00Z"},
		{SessionID: "s3", StartTime: "2026-01-19T10:00:00Z"},
		{SessionID: "s4", StartTime: "2026-01-26T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result.Patterns))
	}

	p := result.Patterns[0]
	if !p.Stale {
		t.Error("expected stale=true for 4 consecutive stable weeks")
	}
	if p.ConsecutiveWeeks != 4 {
		t.Errorf("expected 4 consecutive weeks, got %d", p.ConsecutiveWeeks)
	}
	if p.WeeklyTrend != "stable" {
		t.Errorf("expected trend 'stable', got %q", p.WeeklyTrend)
	}
	if result.StaleCount != 1 {
		t.Errorf("expected stale count 1, got %d", result.StaleCount)
	}
}

func TestAnalyzeFrictionPersistence_SortOrder(t *testing.T) {
	// Two friction types: one stale, one not. Stale should come first.
	facets := []claude.SessionFacet{
		// Stale pattern: present every week, stable.
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s3", FrictionCounts: map[string]int{"wrong_approach": 1}},
		// Non-stale: only appears in week 3.
		{SessionID: "s3", FrictionCounts: map[string]int{"timeout": 1}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-12T10:00:00Z"},
		{SessionID: "s3", StartTime: "2026-01-19T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) < 2 {
		t.Fatalf("expected at least 2 patterns, got %d", len(result.Patterns))
	}

	if result.Patterns[0].FrictionType != "wrong_approach" {
		t.Errorf("expected stale 'wrong_approach' first, got %q", result.Patterns[0].FrictionType)
	}
}

func TestAnalyzeFrictionPersistence_MultipleFrictionTypes(t *testing.T) {
	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 2, "timeout": 1}},
		{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 1}},
	}
	metas := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-06T10:00:00Z"},
	}

	result := AnalyzeFrictionPersistence(facets, metas)
	if len(result.Patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(result.Patterns))
	}

	// wrong_approach appears in 2/2 sessions, timeout in 1/2.
	found := make(map[string]FrictionPersistence)
	for _, p := range result.Patterns {
		found[p.FrictionType] = p
	}

	wa := found["wrong_approach"]
	if wa.OccurrenceCount != 2 {
		t.Errorf("wrong_approach: expected 2 occurrences, got %d", wa.OccurrenceCount)
	}
	if wa.Frequency != 1.0 {
		t.Errorf("wrong_approach: expected frequency 1.0, got %f", wa.Frequency)
	}

	to := found["timeout"]
	if to.OccurrenceCount != 1 {
		t.Errorf("timeout: expected 1 occurrence, got %d", to.OccurrenceCount)
	}
	if to.Frequency != 0.5 {
		t.Errorf("timeout: expected frequency 0.5, got %f", to.Frequency)
	}
}

func TestComputeTrend(t *testing.T) {
	tests := []struct {
		name     string
		counts   []int
		expected string
	}{
		{"single week", []int{3}, "stable"},
		{"two equal weeks", []int{3, 3}, "stable"},
		{"two improving", []int{5, 2}, "improving"},
		{"two worsening", []int{2, 5}, "worsening"},
		{"four weeks improving", []int{5, 5, 2, 2}, "improving"},
		{"four weeks worsening", []int{2, 2, 5, 5}, "worsening"},
		{"four weeks stable", []int{3, 3, 3, 3}, "stable"},
		{"empty", []int{}, "stable"},
		{"within 10 percent", []int{10, 10, 10, 11}, "stable"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeTrend(tc.counts)
			if got != tc.expected {
				t.Errorf("computeTrend(%v) = %q, want %q", tc.counts, got, tc.expected)
			}
		})
	}
}

func TestConsecutiveWeeksFromEnd(t *testing.T) {
	allWeeks := [][2]int{{2026, 1}, {2026, 2}, {2026, 3}, {2026, 4}}

	tests := []struct {
		name     string
		present  map[[2]int]bool
		expected int
	}{
		{
			"all present",
			map[[2]int]bool{{2026, 1}: true, {2026, 2}: true, {2026, 3}: true, {2026, 4}: true},
			4,
		},
		{
			"last two present",
			map[[2]int]bool{{2026, 3}: true, {2026, 4}: true},
			2,
		},
		{
			"gap in middle",
			map[[2]int]bool{{2026, 1}: true, {2026, 4}: true},
			1,
		},
		{
			"none present",
			map[[2]int]bool{},
			0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := consecutiveWeeksFromEnd(allWeeks, tc.present)
			if got != tc.expected {
				t.Errorf("consecutiveWeeksFromEnd = %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestWeeksBetween(t *testing.T) {
	// Same week should return 1 entry.
	weeks := weeksBetween(
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
	)
	if len(weeks) != 1 {
		t.Errorf("same week: expected 1, got %d", len(weeks))
	}

	// Two weeks apart.
	weeks = weeksBetween(
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC),
	)
	if len(weeks) != 2 {
		t.Errorf("two weeks: expected 2, got %d", len(weeks))
	}

	// Reversed times should return nil.
	weeks = weeksBetween(
		time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	)
	if weeks != nil {
		t.Errorf("reversed: expected nil, got %v", weeks)
	}
}
