package analyzer

import (
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// AnalyzeVelocity computes productivity metrics from session data, filtered
// to the last N days. If days is 0 or negative, all sessions are included.
func AnalyzeVelocity(sessions []claude.SessionMeta, days int) VelocityMetrics {
	filtered := filterSessionsByDays(sessions, days)

	metrics := VelocityMetrics{
		TotalSessions: len(filtered),
	}

	if len(filtered) == 0 {
		return metrics
	}

	var totalLines, totalCommits, totalFiles, totalDuration, totalMessages int

	for _, s := range filtered {
		totalLines += s.LinesAdded
		totalCommits += s.GitCommits
		totalFiles += s.FilesModified
		totalDuration += s.DurationMinutes
		totalMessages += s.UserMessageCount + s.AssistantMessageCount
	}

	n := float64(len(filtered))
	metrics.AvgLinesAddedPerSession = float64(totalLines) / n
	metrics.AvgCommitsPerSession = float64(totalCommits) / n
	metrics.AvgFilesModifiedPerSession = float64(totalFiles) / n
	metrics.AvgDurationMinutes = float64(totalDuration) / n
	metrics.AvgMessagesPerSession = float64(totalMessages) / n

	return metrics
}

// filterSessionsByDays returns sessions whose StartTime falls within the last
// N days. If days <= 0, all sessions are returned.
func filterSessionsByDays(sessions []claude.SessionMeta, days int) []claude.SessionMeta {
	if days <= 0 {
		return sessions
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var filtered []claude.SessionMeta

	for _, s := range sessions {
		t := claude.ParseTimestamp(s.StartTime)
		if t.IsZero() {
			continue
		}
		if t.After(cutoff) {
			filtered = append(filtered, s)
		}
	}

	return filtered
}
