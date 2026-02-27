package analyzer

import (
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestAnalyzeCommits_Empty(t *testing.T) {
	result := AnalyzeCommits(nil)
	if result.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions, got %d", result.TotalSessions)
	}
	if result.SessionsWithCommits != 0 {
		t.Errorf("expected 0 sessions with commits, got %d", result.SessionsWithCommits)
	}
	if result.SessionsZeroCommits != 0 {
		t.Errorf("expected 0 zero-commit sessions, got %d", result.SessionsZeroCommits)
	}
	if result.ZeroCommitRate != 0 {
		t.Errorf("expected 0 zero-commit rate, got %f", result.ZeroCommitRate)
	}
	if len(result.ZeroCommitSessions) != 0 {
		t.Errorf("expected 0 zero-commit session details, got %d", len(result.ZeroCommitSessions))
	}
	if len(result.WeeklyCommitRates) != 0 {
		t.Errorf("expected 0 weekly rates, got %d", len(result.WeeklyCommitRates))
	}
}

func TestAnalyzeCommits_AllWithCommits(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z", GitCommits: 2, ProjectPath: "/proj"},
		{SessionID: "s2", StartTime: "2026-01-06T10:00:00Z", GitCommits: 3, ProjectPath: "/proj"},
		{SessionID: "s3", StartTime: "2026-01-07T10:00:00Z", GitCommits: 1, ProjectPath: "/proj"},
	}

	result := AnalyzeCommits(sessions)

	if result.TotalSessions != 3 {
		t.Errorf("expected 3 total sessions, got %d", result.TotalSessions)
	}
	if result.SessionsWithCommits != 3 {
		t.Errorf("expected 3 sessions with commits, got %d", result.SessionsWithCommits)
	}
	if result.SessionsZeroCommits != 0 {
		t.Errorf("expected 0 zero-commit sessions, got %d", result.SessionsZeroCommits)
	}
	if result.ZeroCommitRate != 0.0 {
		t.Errorf("expected zero-commit rate 0.0, got %f", result.ZeroCommitRate)
	}

	// AvgCommitsPerSession = (2+3+1)/3 = 2.0
	wantAvg := 2.0
	if diff := result.AvgCommitsPerSession - wantAvg; diff > 0.001 || diff < -0.001 {
		t.Errorf("AvgCommitsPerSession = %.4f, want %.4f", result.AvgCommitsPerSession, wantAvg)
	}

	if result.MaxCommitsInSession != 3 {
		t.Errorf("expected max commits 3, got %d", result.MaxCommitsInSession)
	}
}

func TestAnalyzeCommits_AllZeroCommits(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID:             "s1",
			StartTime:             "2026-01-05T10:00:00Z",
			GitCommits:            0,
			ProjectPath:           "/proj/alpha",
			DurationMinutes:       30,
			UserMessageCount:      5,
			AssistantMessageCount: 5,
			ToolCounts:            map[string]int{"Read": 10, "Bash": 5, "Edit": 2},
		},
		{
			SessionID:             "s2",
			StartTime:             "2026-01-06T10:00:00Z",
			GitCommits:            0,
			ProjectPath:           "/proj/beta",
			DurationMinutes:       60,
			UserMessageCount:      10,
			AssistantMessageCount: 10,
			ToolCounts:            map[string]int{"Read": 20, "Bash": 15},
		},
	}

	result := AnalyzeCommits(sessions)

	if result.SessionsZeroCommits != 2 {
		t.Errorf("expected 2 zero-commit sessions, got %d", result.SessionsZeroCommits)
	}
	if result.ZeroCommitRate != 1.0 {
		t.Errorf("expected zero-commit rate 1.0, got %f", result.ZeroCommitRate)
	}
	if result.AvgCommitsPerSession != 0 {
		t.Errorf("expected avg commits 0, got %f", result.AvgCommitsPerSession)
	}

	// Zero-commit sessions should be sorted by duration descending.
	if len(result.ZeroCommitSessions) != 2 {
		t.Fatalf("expected 2 zero-commit session details, got %d", len(result.ZeroCommitSessions))
	}
	if result.ZeroCommitSessions[0].Duration != 60 {
		t.Errorf("expected first zero-commit session duration=60 (longest), got %d", result.ZeroCommitSessions[0].Duration)
	}
	if result.ZeroCommitSessions[1].Duration != 30 {
		t.Errorf("expected second zero-commit session duration=30, got %d", result.ZeroCommitSessions[1].Duration)
	}
}

func TestAnalyzeCommits_ZeroCommitSessionDetails(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID:             "s1",
			StartTime:             "2026-01-05T10:00:00Z",
			GitCommits:            0,
			ProjectPath:           "/home/user/projects/myapp",
			DurationMinutes:       45,
			UserMessageCount:      8,
			AssistantMessageCount: 12,
			ToolCounts:            map[string]int{"Read": 20, "Bash": 10, "Edit": 5, "Grep": 3},
		},
	}

	result := AnalyzeCommits(sessions)

	if len(result.ZeroCommitSessions) != 1 {
		t.Fatalf("expected 1 zero-commit session, got %d", len(result.ZeroCommitSessions))
	}

	zcs := result.ZeroCommitSessions[0]
	if zcs.SessionID != "s1" {
		t.Errorf("expected session ID 's1', got %q", zcs.SessionID)
	}
	if zcs.Duration != 45 {
		t.Errorf("expected duration 45, got %d", zcs.Duration)
	}
	if zcs.Messages != 20 { // 8 + 12
		t.Errorf("expected messages 20, got %d", zcs.Messages)
	}
	if zcs.ProjectName != "myapp" {
		t.Errorf("expected project name 'myapp', got %q", zcs.ProjectName)
	}

	// Top 3 tools: Read (20), Bash (10), Edit (5)
	if len(zcs.TopTools) != 3 {
		t.Fatalf("expected 3 top tools, got %d", len(zcs.TopTools))
	}
	if zcs.TopTools[0] != "Read" {
		t.Errorf("expected top tool 'Read', got %q", zcs.TopTools[0])
	}
	if zcs.TopTools[1] != "Bash" {
		t.Errorf("expected second tool 'Bash', got %q", zcs.TopTools[1])
	}
	if zcs.TopTools[2] != "Edit" {
		t.Errorf("expected third tool 'Edit', got %q", zcs.TopTools[2])
	}
}

func TestAnalyzeCommits_MixedCommits(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z", GitCommits: 3, ProjectPath: "/proj"},
		{SessionID: "s2", StartTime: "2026-01-06T10:00:00Z", GitCommits: 0, ProjectPath: "/proj", DurationMinutes: 20},
		{SessionID: "s3", StartTime: "2026-01-07T10:00:00Z", GitCommits: 1, ProjectPath: "/proj"},
		{SessionID: "s4", StartTime: "2026-01-08T10:00:00Z", GitCommits: 0, ProjectPath: "/proj", DurationMinutes: 10},
	}

	result := AnalyzeCommits(sessions)

	if result.TotalSessions != 4 {
		t.Errorf("expected 4 total sessions, got %d", result.TotalSessions)
	}
	if result.SessionsWithCommits != 2 {
		t.Errorf("expected 2 sessions with commits, got %d", result.SessionsWithCommits)
	}
	if result.SessionsZeroCommits != 2 {
		t.Errorf("expected 2 zero-commit sessions, got %d", result.SessionsZeroCommits)
	}

	wantRate := 0.5
	if diff := result.ZeroCommitRate - wantRate; diff > 0.001 || diff < -0.001 {
		t.Errorf("ZeroCommitRate = %.4f, want %.4f", result.ZeroCommitRate, wantRate)
	}

	// AvgCommitsPerSession = 4 / 4 = 1.0
	if diff := result.AvgCommitsPerSession - 1.0; diff > 0.001 || diff < -0.001 {
		t.Errorf("AvgCommitsPerSession = %.4f, want 1.0", result.AvgCommitsPerSession)
	}

	if result.MaxCommitsInSession != 3 {
		t.Errorf("expected max commits 3, got %d", result.MaxCommitsInSession)
	}
}

func TestAnalyzeCommits_WeeklyCommitRates(t *testing.T) {
	sessions := []claude.SessionMeta{
		// Week of Jan 5 (Monday)
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z", GitCommits: 1, ProjectPath: "/proj"},
		{SessionID: "s2", StartTime: "2026-01-06T10:00:00Z", GitCommits: 0, ProjectPath: "/proj"},
		// Week of Jan 12 (Monday)
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", GitCommits: 2, ProjectPath: "/proj"},
		{SessionID: "s4", StartTime: "2026-01-14T10:00:00Z", GitCommits: 1, ProjectPath: "/proj"},
		// Week of Jan 19 (Monday)
		{SessionID: "s5", StartTime: "2026-01-19T10:00:00Z", GitCommits: 0, ProjectPath: "/proj"},
	}

	result := AnalyzeCommits(sessions)

	if len(result.WeeklyCommitRates) != 3 {
		t.Fatalf("expected 3 weekly buckets, got %d", len(result.WeeklyCommitRates))
	}

	// Should be sorted chronologically.
	for i := 1; i < len(result.WeeklyCommitRates); i++ {
		if result.WeeklyCommitRates[i].WeekStart.Before(result.WeeklyCommitRates[i-1].WeekStart) {
			t.Error("weekly rates should be sorted chronologically")
		}
	}

	// Week 1 (Jan 5): 2 sessions, 1 with commits -> rate 0.5
	w1 := result.WeeklyCommitRates[0]
	if w1.Sessions != 2 {
		t.Errorf("week 1: expected 2 sessions, got %d", w1.Sessions)
	}
	if w1.WithCommits != 1 {
		t.Errorf("week 1: expected 1 with commits, got %d", w1.WithCommits)
	}
	wantW1Rate := 0.5
	if diff := w1.Rate - wantW1Rate; diff > 0.001 || diff < -0.001 {
		t.Errorf("week 1: rate = %.4f, want %.4f", w1.Rate, wantW1Rate)
	}

	// Week 2 (Jan 12): 2 sessions, 2 with commits -> rate 1.0
	w2 := result.WeeklyCommitRates[1]
	if w2.Sessions != 2 {
		t.Errorf("week 2: expected 2 sessions, got %d", w2.Sessions)
	}
	if w2.WithCommits != 2 {
		t.Errorf("week 2: expected 2 with commits, got %d", w2.WithCommits)
	}

	// Week 3 (Jan 19): 1 session, 0 with commits -> rate 0.0
	w3 := result.WeeklyCommitRates[2]
	if w3.Sessions != 1 {
		t.Errorf("week 3: expected 1 session, got %d", w3.Sessions)
	}
	if w3.WithCommits != 0 {
		t.Errorf("week 3: expected 0 with commits, got %d", w3.WithCommits)
	}
	if w3.Rate != 0.0 {
		t.Errorf("week 3: rate = %.4f, want 0.0", w3.Rate)
	}
}

func TestTopNTools(t *testing.T) {
	tests := []struct {
		name  string
		tools map[string]int
		n     int
		want  []string
	}{
		{
			name:  "top 3 from 4",
			tools: map[string]int{"Read": 20, "Bash": 10, "Edit": 5, "Grep": 3},
			n:     3,
			want:  []string{"Read", "Bash", "Edit"},
		},
		{
			name:  "top 3 from 2",
			tools: map[string]int{"Read": 10, "Bash": 5},
			n:     3,
			want:  []string{"Read", "Bash"},
		},
		{
			name:  "empty",
			tools: map[string]int{},
			n:     3,
			want:  []string{},
		},
		{
			name:  "nil map",
			tools: nil,
			n:     3,
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topNTools(tt.tools, tt.n)
			if len(got) != len(tt.want) {
				t.Errorf("topNTools() returned %d tools, want %d", len(got), len(tt.want))
				return
			}
			for i, tool := range tt.want {
				if got[i] != tool {
					t.Errorf("topNTools()[%d] = %q, want %q", i, got[i], tool)
				}
			}
		})
	}
}

func TestWeekStartMonday(t *testing.T) {
	tests := []struct {
		name    string
		input   time.Time
		wantDay int // day of month of the expected Monday
	}{
		{
			name:    "monday stays monday",
			input:   time.Date(2026, 1, 5, 15, 30, 0, 0, time.UTC),
			wantDay: 5,
		},
		{
			name:    "wednesday goes to monday",
			input:   time.Date(2026, 1, 7, 10, 0, 0, 0, time.UTC),
			wantDay: 5,
		},
		{
			name:    "sunday goes to monday",
			input:   time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
			wantDay: 5,
		},
		{
			name:    "saturday goes to monday",
			input:   time.Date(2026, 1, 10, 10, 0, 0, 0, time.UTC),
			wantDay: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monday := weekStartMonday(tt.input)
			if monday.Weekday() != time.Monday {
				t.Errorf("weekStartMonday() returned %s, want Monday", monday.Weekday())
			}
			if monday.Day() != tt.wantDay {
				t.Errorf("weekStartMonday() day = %d, want %d", monday.Day(), tt.wantDay)
			}
			// Should be at midnight UTC.
			if monday.Hour() != 0 || monday.Minute() != 0 || monday.Second() != 0 {
				t.Errorf("weekStartMonday() not at midnight: %v", monday)
			}
		})
	}
}

func TestParseTimestamp_FromCommits(t *testing.T) {
	tests := []struct {
		name  string
		input string
		zero  bool
	}{
		{"RFC3339", "2026-01-05T10:00:00Z", false},
		{"RFC3339Nano", "2026-01-05T10:00:00.123Z", false},
		{"plain datetime", "2026-01-05T10:00:00", false},
		{"empty", "", true},
		{"invalid", "not-a-time", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := claude.ParseTimestamp(tt.input)
			if tt.zero && !result.IsZero() {
				t.Errorf("expected zero time for %q, got %v", tt.input, result)
			}
			if !tt.zero && result.IsZero() {
				t.Errorf("expected non-zero time for %q", tt.input)
			}
		})
	}
}

func TestAnalyzeCommits_ZeroCommitSessionsSortedByDuration(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "short", StartTime: "2026-01-05T10:00:00Z", GitCommits: 0, DurationMinutes: 5, ProjectPath: "/p"},
		{SessionID: "long", StartTime: "2026-01-05T11:00:00Z", GitCommits: 0, DurationMinutes: 120, ProjectPath: "/p"},
		{SessionID: "medium", StartTime: "2026-01-05T12:00:00Z", GitCommits: 0, DurationMinutes: 30, ProjectPath: "/p"},
	}

	result := AnalyzeCommits(sessions)

	if len(result.ZeroCommitSessions) != 3 {
		t.Fatalf("expected 3 zero-commit sessions, got %d", len(result.ZeroCommitSessions))
	}

	// Should be sorted by duration descending.
	if result.ZeroCommitSessions[0].SessionID != "long" {
		t.Errorf("expected first session 'long', got %q", result.ZeroCommitSessions[0].SessionID)
	}
	if result.ZeroCommitSessions[1].SessionID != "medium" {
		t.Errorf("expected second session 'medium', got %q", result.ZeroCommitSessions[1].SessionID)
	}
	if result.ZeroCommitSessions[2].SessionID != "short" {
		t.Errorf("expected third session 'short', got %q", result.ZeroCommitSessions[2].SessionID)
	}
}

func TestAnalyzeCommits_SingleSession(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-05T10:00:00Z", GitCommits: 5, ProjectPath: "/proj"},
	}

	result := AnalyzeCommits(sessions)

	if result.TotalSessions != 1 {
		t.Errorf("expected 1 total session, got %d", result.TotalSessions)
	}
	if result.SessionsWithCommits != 1 {
		t.Errorf("expected 1 session with commits, got %d", result.SessionsWithCommits)
	}
	if result.MaxCommitsInSession != 5 {
		t.Errorf("expected max commits 5, got %d", result.MaxCommitsInSession)
	}
	if result.AvgCommitsPerSession != 5.0 {
		t.Errorf("expected avg commits 5.0, got %f", result.AvgCommitsPerSession)
	}

	if len(result.WeeklyCommitRates) != 1 {
		t.Fatalf("expected 1 weekly bucket, got %d", len(result.WeeklyCommitRates))
	}
	if result.WeeklyCommitRates[0].Rate != 1.0 {
		t.Errorf("expected weekly rate 1.0, got %f", result.WeeklyCommitRates[0].Rate)
	}
}
