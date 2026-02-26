package analyzer

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// CommitAnalysis captures commit-to-session ratio metrics and zero-commit
// session details. Zero-commit sessions represent "planning without building"
// — the tool usage within those sessions reveals whether the user was
// exploring (heavy Read/Grep) or building without committing (heavy Bash).
type CommitAnalysis struct {
	// TotalSessions is the number of sessions analyzed.
	TotalSessions int `json:"total_sessions"`

	// SessionsWithCommits is the count of sessions with at least one commit.
	SessionsWithCommits int `json:"sessions_with_commits"`

	// SessionsZeroCommits is the count of sessions with zero commits.
	SessionsZeroCommits int `json:"sessions_zero_commits"`

	// ZeroCommitRate is the fraction of sessions with zero commits.
	ZeroCommitRate float64 `json:"zero_commit_rate"`

	// AvgCommitsPerSession is the mean git commits across all sessions.
	AvgCommitsPerSession float64 `json:"avg_commits_per_session"`

	// MaxCommitsInSession is the highest commit count in any single session.
	MaxCommitsInSession int `json:"max_commits_in_session"`

	// ZeroCommitSessions lists details for each zero-commit session,
	// sorted by duration descending (longest wasted sessions first).
	ZeroCommitSessions []ZeroCommitSession `json:"zero_commit_sessions"`

	// WeeklyCommitRates tracks the commit rate by week for trend analysis.
	WeeklyCommitRates []WeeklyCommitRate `json:"weekly_commit_rates"`
}

// ZeroCommitSession captures details about a session that produced no commits.
type ZeroCommitSession struct {
	// SessionID is the unique identifier for this session.
	SessionID string `json:"session_id"`

	// Date is the parsed start time of the session.
	Date time.Time `json:"date"`

	// Duration is the session length in minutes.
	Duration int `json:"duration"`

	// Messages is the total message count (user + assistant).
	Messages int `json:"messages"`

	// TopTools lists up to 3 most-used tools, showing what the session
	// was doing instead of committing.
	TopTools []string `json:"top_tools"`

	// ProjectName is the short directory name of the associated project.
	ProjectName string `json:"project_name"`
}

// WeeklyCommitRate captures the commit rate for a single ISO week.
type WeeklyCommitRate struct {
	// WeekStart is the Monday of the week.
	WeekStart time.Time `json:"week_start"`

	// Sessions is the total session count for the week.
	Sessions int `json:"sessions"`

	// WithCommits is the count of sessions that had at least one commit.
	WithCommits int `json:"with_commits"`

	// Rate is the fraction of sessions with commits (WithCommits / Sessions).
	Rate float64 `json:"rate"`
}

// AnalyzeCommits computes commit-to-session ratio metrics and identifies
// zero-commit sessions from the provided session metadata.
func AnalyzeCommits(sessions []claude.SessionMeta) CommitAnalysis {
	analysis := CommitAnalysis{
		TotalSessions: len(sessions),
	}

	if len(sessions) == 0 {
		return analysis
	}

	// weekKey maps a session's start time to the Monday of its ISO week.
	// Weekly buckets are keyed by this Monday date string.
	weekBuckets := make(map[string]*weekBucket)

	var totalCommits int

	for _, s := range sessions {
		t := claude.ParseTimestamp(s.StartTime)

		totalCommits += s.GitCommits

		if s.GitCommits > analysis.MaxCommitsInSession {
			analysis.MaxCommitsInSession = s.GitCommits
		}

		if s.GitCommits > 0 {
			analysis.SessionsWithCommits++
		} else {
			zcs := ZeroCommitSession{
				SessionID:   s.SessionID,
				Date:        t,
				Duration:    s.DurationMinutes,
				Messages:    s.UserMessageCount + s.AssistantMessageCount,
				TopTools:    topNTools(s.ToolCounts, 3),
				ProjectName: filepath.Base(s.ProjectPath),
			}
			analysis.ZeroCommitSessions = append(analysis.ZeroCommitSessions, zcs)
		}

		// Bucket into weekly slots.
		monday := weekStartMonday(t)
		key := monday.Format("2006-01-02")
		wb, ok := weekBuckets[key]
		if !ok {
			wb = &weekBucket{weekStart: monday}
			weekBuckets[key] = wb
		}
		wb.sessions++
		if s.GitCommits > 0 {
			wb.withCommits++
		}
	}

	analysis.SessionsZeroCommits = analysis.TotalSessions - analysis.SessionsWithCommits
	n := float64(analysis.TotalSessions)
	analysis.AvgCommitsPerSession = float64(totalCommits) / n

	if analysis.TotalSessions > 0 {
		analysis.ZeroCommitRate = float64(analysis.SessionsZeroCommits) / n
	}

	// Sort zero-commit sessions by duration descending.
	sort.Slice(analysis.ZeroCommitSessions, func(i, j int) bool {
		return analysis.ZeroCommitSessions[i].Duration > analysis.ZeroCommitSessions[j].Duration
	})

	// Build sorted weekly commit rates.
	analysis.WeeklyCommitRates = buildWeeklyRates(weekBuckets)

	return analysis
}

// weekBucket accumulates session counts for a single week.
type weekBucket struct {
	weekStart   time.Time
	sessions    int
	withCommits int
}

// buildWeeklyRates converts the week bucket map into a sorted slice of
// WeeklyCommitRate, ordered by week start ascending.
func buildWeeklyRates(buckets map[string]*weekBucket) []WeeklyCommitRate {
	rates := make([]WeeklyCommitRate, 0, len(buckets))

	for _, wb := range buckets {
		rate := 0.0
		if wb.sessions > 0 {
			rate = float64(wb.withCommits) / float64(wb.sessions)
		}
		rates = append(rates, WeeklyCommitRate{
			WeekStart:   wb.weekStart,
			Sessions:    wb.sessions,
			WithCommits: wb.withCommits,
			Rate:        rate,
		})
	}

	sort.Slice(rates, func(i, j int) bool {
		return rates[i].WeekStart.Before(rates[j].WeekStart)
	})

	return rates
}

// topNTools returns the names of the top N tools by usage count from the
// provided tool counts map.
func topNTools(toolCounts map[string]int, n int) []string {
	type toolEntry struct {
		name  string
		count int
	}

	entries := make([]toolEntry, 0, len(toolCounts))
	for name, count := range toolCounts {
		entries = append(entries, toolEntry{name: name, count: count})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	limit := n
	if limit > len(entries) {
		limit = len(entries)
	}

	tools := make([]string, limit)
	for i := 0; i < limit; i++ {
		tools[i] = entries[i].name
	}

	return tools
}

// weekStartMonday returns the Monday 00:00:00 UTC for the ISO week
// containing the given time.
func weekStartMonday(t time.Time) time.Time {
	t = t.UTC()
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday := t.AddDate(0, 0, -int(weekday-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

