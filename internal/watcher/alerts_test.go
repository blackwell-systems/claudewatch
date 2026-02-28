package watcher

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func makeState() *WatchState {
	return &WatchState{
		FrictionCounts: make(map[string]int),
		frictionByType: make(map[string]int),
		sessions:       nil,
	}
}

func TestCompare_NoChanges(t *testing.T) {
	prev := makeState()
	prev.SessionCount = 5
	prev.FrictionCounts["wrong_approach"] = 3
	prev.frictionByType["wrong_approach"] = 3

	curr := makeState()
	curr.SessionCount = 5
	curr.FrictionCounts["wrong_approach"] = 3
	curr.frictionByType["wrong_approach"] = 3

	alerts := Compare(prev, curr)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts for identical states, got %d", len(alerts))
		for _, a := range alerts {
			t.Logf("  [%s] %s: %s", a.Level, a.Title, a.Message)
		}
	}
}

func TestCompare_IdenticalStates(t *testing.T) {
	prev := makeState()
	curr := makeState()

	alerts := Compare(prev, curr)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts for empty identical states, got %d", len(alerts))
	}
}

func TestCompare_NewSession(t *testing.T) {
	prev := makeState()
	prev.SessionCount = 1
	prev.sessions = []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/tmp/proj", DurationMinutes: 10, GitCommits: 2, ToolCounts: map[string]int{"Bash": 5}},
	}

	curr := makeState()
	curr.SessionCount = 2
	curr.sessions = []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/tmp/proj", DurationMinutes: 10, GitCommits: 2, ToolCounts: map[string]int{"Bash": 5}},
		{SessionID: "s2", ProjectPath: "/tmp/proj", DurationMinutes: 15, GitCommits: 1, ToolCounts: map[string]int{"Read": 3}},
	}

	alerts := Compare(prev, curr)

	hasInfoSession := false
	for _, a := range alerts {
		if a.Level == "info" && a.Title != "" {
			hasInfoSession = true
		}
	}
	if !hasInfoSession {
		t.Error("expected info alert for new session")
	}
}

func TestCompare_NewFrictionType(t *testing.T) {
	prev := makeState()
	prev.FrictionCounts["wrong_approach"] = 3

	curr := makeState()
	curr.FrictionCounts["wrong_approach"] = 3
	curr.FrictionCounts["scope_creep"] = 2

	alerts := Compare(prev, curr)

	hasWarning := false
	for _, a := range alerts {
		if a.Level == "warning" && a.Title == "New friction type: scope_creep" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning alert for new friction type")
	}
}

func TestCompare_FrictionFrequencyIncrease(t *testing.T) {
	prev := makeState()
	prev.FrictionCounts["wrong_approach"] = 10

	curr := makeState()
	curr.FrictionCounts["wrong_approach"] = 15 // 50% increase

	alerts := Compare(prev, curr)

	hasSpike := false
	for _, a := range alerts {
		if a.Level == "warning" && a.Title == "Friction spike: wrong_approach" {
			hasSpike = true
		}
	}
	if !hasSpike {
		t.Error("expected warning alert for friction frequency increase >20%")
	}
}

func TestCompare_FrictionFrequencySmallIncrease(t *testing.T) {
	prev := makeState()
	prev.FrictionCounts["wrong_approach"] = 10

	curr := makeState()
	curr.FrictionCounts["wrong_approach"] = 11 // 10% increase, below threshold

	alerts := Compare(prev, curr)

	for _, a := range alerts {
		if a.Level == "warning" && a.Title == "Friction spike: wrong_approach" {
			t.Error("should not alert for small friction increase (<= 20%)")
		}
	}
}

func TestCompare_NewStalePattern(t *testing.T) {
	prev := makeState()
	prev.StalePatterns = 0
	prev.persistence = analyzer.PersistenceAnalysis{
		StaleCount: 0,
		Patterns:   nil,
	}

	curr := makeState()
	curr.StalePatterns = 1
	curr.persistence = analyzer.PersistenceAnalysis{
		StaleCount: 1,
		Patterns: []analyzer.FrictionPersistence{
			{
				FrictionType:     "wrong_approach",
				Stale:            true,
				ConsecutiveWeeks: 4,
				OccurrenceCount:  10,
			},
		},
	}

	alerts := Compare(prev, curr)

	hasCritical := false
	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "Stale friction: wrong_approach" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected critical alert for new stale pattern")
	}
}

func TestCompare_AgentKillRateSpike(t *testing.T) {
	prev := makeState()
	prev.agentKillRate = 0.20
	prev.AgentCount = 5

	curr := makeState()
	curr.agentKillRate = 0.45
	curr.AgentCount = 10

	alerts := Compare(prev, curr)

	hasCritical := false
	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "Agent kill rate spike" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected critical alert for agent kill rate spike above 30%")
	}
}

func TestCompare_AgentKillRateNoSpike(t *testing.T) {
	prev := makeState()
	prev.agentKillRate = 0.35 // already above threshold
	prev.AgentCount = 5

	curr := makeState()
	curr.agentKillRate = 0.40
	curr.AgentCount = 10

	alerts := Compare(prev, curr)

	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "Agent kill rate spike" {
			t.Error("should not alert when kill rate was already above 30%")
		}
	}
}

func TestCompare_ZeroCommitRateHigh(t *testing.T) {
	// Need at least 5 recent non-trivial sessions with >80% zero commits.
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", GitCommits: 0, DurationMinutes: 30},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", GitCommits: 0, DurationMinutes: 20},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", GitCommits: 0, DurationMinutes: 15},
		{SessionID: "s4", StartTime: "2026-01-13T10:00:00Z", GitCommits: 0, DurationMinutes: 25},
		{SessionID: "s5", StartTime: "2026-01-14T10:00:00Z", GitCommits: 0, DurationMinutes: 10},
	}

	prev := makeState()
	curr := makeState()
	curr.sessions = sessions
	curr.SessionCount = 5

	alerts := Compare(prev, curr)

	hasCritical := false
	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "High zero-commit rate" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected critical alert for >80% zero-commit rate in last 5 sessions")
	}
}

func TestCompare_ZeroCommitRateAcceptable(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", GitCommits: 0, DurationMinutes: 20},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", GitCommits: 2, DurationMinutes: 30},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", GitCommits: 0, DurationMinutes: 15},
		{SessionID: "s4", StartTime: "2026-01-13T10:00:00Z", GitCommits: 1, DurationMinutes: 25},
		{SessionID: "s5", StartTime: "2026-01-14T10:00:00Z", GitCommits: 3, DurationMinutes: 40},
	}

	prev := makeState()
	curr := makeState()
	curr.sessions = sessions
	curr.SessionCount = 5

	alerts := Compare(prev, curr)

	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "High zero-commit rate" {
			t.Error("should not alert when zero-commit rate is 40% (below 80%)")
		}
	}
}

func TestCompare_FrictionImprovement(t *testing.T) {
	prev := makeState()
	prev.FrictionCounts["wrong_approach"] = 10

	curr := makeState()
	curr.FrictionCounts["wrong_approach"] = 5 // 50% decrease

	alerts := Compare(prev, curr)

	hasInfo := false
	for _, a := range alerts {
		if a.Level == "info" && a.Title == "Friction improved: wrong_approach" {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Error("expected info alert for friction improvement (>20% decrease)")
	}
}

func TestCompare_FrictionImprovementSmallDecrease(t *testing.T) {
	prev := makeState()
	prev.FrictionCounts["wrong_approach"] = 10

	curr := makeState()
	curr.FrictionCounts["wrong_approach"] = 9 // 10% decrease, below threshold

	alerts := Compare(prev, curr)

	for _, a := range alerts {
		if a.Level == "info" && a.Title == "Friction improved: wrong_approach" {
			t.Error("should not alert for small friction decrease (<= 20%)")
		}
	}
}

func TestCompare_StalePatternResolved(t *testing.T) {
	prev := makeState()
	prev.StalePatterns = 2

	curr := makeState()
	curr.StalePatterns = 1

	alerts := Compare(prev, curr)

	hasInfo := false
	for _, a := range alerts {
		if a.Level == "info" && a.Title == "Stale friction resolved" {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Error("expected info alert for stale pattern resolution")
	}
}

func TestCompare_AgentSuccessRateDrop(t *testing.T) {
	prev := makeState()
	prev.agentSuccessRate = 0.90
	prev.AgentCount = 10

	curr := makeState()
	curr.agentSuccessRate = 0.70
	curr.AgentCount = 15

	alerts := Compare(prev, curr)

	hasWarning := false
	for _, a := range alerts {
		if a.Level == "warning" && a.Title == "Agent success rate dropped" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning alert for agent success rate drop below 80%")
	}
}

func TestFindNewSessions(t *testing.T) {
	prev := &WatchState{
		sessions: []claude.SessionMeta{
			{SessionID: "s1"},
			{SessionID: "s2"},
		},
	}

	curr := &WatchState{
		sessions: []claude.SessionMeta{
			{SessionID: "s1"},
			{SessionID: "s2"},
			{SessionID: "s3", ProjectPath: "/tmp/proj", DurationMinutes: 20, GitCommits: 3},
		},
	}

	newSessions := findNewSessions(prev, curr)
	if len(newSessions) != 1 {
		t.Fatalf("expected 1 new session, got %d", len(newSessions))
	}
	if newSessions[0].SessionID != "s3" {
		t.Errorf("expected new session s3, got %s", newSessions[0].SessionID)
	}
}

func TestRecentSessions(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-15T10:00:00Z"},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z"},
	}

	recent := recentSessions(sessions, 2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent sessions, got %d", len(recent))
	}
	// Should be sorted by start time descending.
	if recent[0].SessionID != "s2" {
		t.Errorf("expected most recent session s2, got %s", recent[0].SessionID)
	}
	if recent[1].SessionID != "s3" {
		t.Errorf("expected second most recent session s3, got %s", recent[1].SessionID)
	}
}

func TestRecentSessions_Empty(t *testing.T) {
	recent := recentSessions(nil, 5)
	if recent != nil {
		t.Errorf("expected nil for empty sessions, got %v", recent)
	}
}

func TestFilterNonTrivialSessions(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "trivial1", DurationMinutes: 3, UserMessageCount: 1},   // trivial: short + few messages
		{SessionID: "long", DurationMinutes: 30, UserMessageCount: 2},      // non-trivial: ≥10 min
		{SessionID: "chatty", DurationMinutes: 5, UserMessageCount: 8},     // non-trivial: ≥5 messages
		{SessionID: "trivial2", DurationMinutes: 2, UserMessageCount: 3},   // trivial
		{SessionID: "both", DurationMinutes: 20, UserMessageCount: 10},     // non-trivial: both
	}

	result := filterNonTrivialSessions(sessions)
	if len(result) != 3 {
		t.Fatalf("expected 3 non-trivial sessions, got %d", len(result))
	}
	ids := map[string]bool{}
	for _, s := range result {
		ids[s.SessionID] = true
	}
	for _, want := range []string{"long", "chatty", "both"} {
		if !ids[want] {
			t.Errorf("expected session %q in non-trivial results", want)
		}
	}
}

func TestCompare_ZeroCommitRateTrivialSessionsFiltered(t *testing.T) {
	// 5 trivial sessions with zero commits should NOT trigger the alert
	// because they get filtered out, leaving < 5 non-trivial sessions.
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", GitCommits: 0, DurationMinutes: 3, UserMessageCount: 1},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", GitCommits: 0, DurationMinutes: 2, UserMessageCount: 2},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", GitCommits: 0, DurationMinutes: 5, UserMessageCount: 1},
		{SessionID: "s4", StartTime: "2026-01-13T10:00:00Z", GitCommits: 0, DurationMinutes: 1, UserMessageCount: 1},
		{SessionID: "s5", StartTime: "2026-01-14T10:00:00Z", GitCommits: 0, DurationMinutes: 4, UserMessageCount: 3},
	}

	prev := makeState()
	curr := makeState()
	curr.sessions = sessions
	curr.SessionCount = 5

	alerts := Compare(prev, curr)

	for _, a := range alerts {
		if a.Level == "critical" && a.Title == "High zero-commit rate" {
			t.Error("should not alert when all sessions are trivial (< 5 messages and < 10 min)")
		}
	}
}

func TestRecentSessions_LargerN(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z"},
	}

	recent := recentSessions(sessions, 10)
	if len(recent) != 1 {
		t.Fatalf("expected 1 session when n > len, got %d", len(recent))
	}
}
