package watcher

import (
	"fmt"
	"path/filepath"
	"time"
)

// Compare detects notable changes between two watch states and returns alerts.
// It checks for critical, warning, and info-level changes.
func Compare(prev, curr *WatchState) []Alert {
	var alerts []Alert

	alerts = append(alerts, compareCritical(prev, curr)...)
	alerts = append(alerts, compareWarning(prev, curr)...)
	alerts = append(alerts, compareInfo(prev, curr)...)

	return alerts
}

// compareCritical detects critical-level changes.
func compareCritical(prev, curr *WatchState) []Alert {
	var alerts []Alert
	now := time.Now()

	// New stale friction pattern appeared (wasn't stale before, now is).
	if curr.StalePatterns > prev.StalePatterns {
		newStale := curr.StalePatterns - prev.StalePatterns
		// Find which patterns are newly stale.
		prevStaleTypes := make(map[string]bool)
		for _, p := range prev.persistence.Patterns {
			if p.Stale {
				prevStaleTypes[p.FrictionType] = true
			}
		}
		for _, p := range curr.persistence.Patterns {
			if p.Stale && !prevStaleTypes[p.FrictionType] {
				alerts = append(alerts, Alert{
					Level:   "critical",
					Title:   fmt.Sprintf("Stale friction: %s", p.FrictionType),
					Message: fmt.Sprintf("Persisted for %d consecutive weeks without improvement (%d occurrences)", p.ConsecutiveWeeks, p.OccurrenceCount),
					Time:    now,
				})
			}
		}
		if len(alerts) == 0 && newStale > 0 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Title:   "New stale friction detected",
				Message: fmt.Sprintf("%d friction pattern(s) now stale (3+ weeks without improvement)", newStale),
				Time:    now,
			})
		}
	}

	// Agent kill rate spiked above 30% in recent sessions.
	if curr.agentKillRate > 0.30 && prev.agentKillRate <= 0.30 && curr.AgentCount > 0 {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Title:   "Agent kill rate spike",
			Message: fmt.Sprintf("Kill rate is %.0f%% (was %.0f%%), suggesting agents are failing or being interrupted", curr.agentKillRate*100, prev.agentKillRate*100),
			Time:    now,
		})
	}

	// Zero-commit rate above 80% over last 5 sessions.
	recent := recentSessions(curr.sessions, 5)
	if len(recent) >= 5 {
		zeroCount := 0
		for _, s := range recent {
			if s.GitCommits == 0 {
				zeroCount++
			}
		}
		zeroRate := float64(zeroCount) / float64(len(recent))
		if zeroRate > 0.80 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Title:   "High zero-commit rate",
				Message: fmt.Sprintf("%.0f%% of last %d sessions produced no commits", zeroRate*100, len(recent)),
				Time:    now,
			})
		}
	}

	return alerts
}

// compareWarning detects warning-level changes.
func compareWarning(prev, curr *WatchState) []Alert {
	var alerts []Alert
	now := time.Now()

	// New friction type appeared that wasn't seen before.
	for frictionType, count := range curr.FrictionCounts {
		if _, existed := prev.FrictionCounts[frictionType]; !existed && count > 0 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Title:   fmt.Sprintf("New friction type: %s", frictionType),
				Message: fmt.Sprintf("First appearance with %d occurrence(s)", count),
				Time:    now,
			})
		}
	}

	// Friction frequency increased by >20% for an existing type.
	for frictionType, currCount := range curr.FrictionCounts {
		prevCount, existed := prev.FrictionCounts[frictionType]
		if !existed || prevCount == 0 {
			continue
		}
		increase := float64(currCount-prevCount) / float64(prevCount)
		if increase > 0.20 && currCount > prevCount {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Title:   fmt.Sprintf("Friction spike: %s", frictionType),
				Message: fmt.Sprintf("Increased from %d to %d (+%.0f%%)", prevCount, currCount, increase*100),
				Time:    now,
			})
		}
	}

	// A session just completed with >5 corrections (high correction rate).
	// Detect by looking at new sessions not present in previous snapshot.
	if curr.SessionCount > prev.SessionCount {
		newSessions := findNewSessions(prev, curr)
		for _, s := range newSessions {
			if s.UserInterruptions > 5 {
				alerts = append(alerts, Alert{
					Level:   "warning",
					Title:   "High correction session",
					Message: fmt.Sprintf("Session in %s had %d interruptions (%.0f min, %d commits)", filepath.Base(s.ProjectPath), s.UserInterruptions, float64(s.DurationMinutes), s.GitCommits),
					Time:    now,
				})
			}
		}
	}

	// Agent success rate dropped below 80%.
	if curr.agentSuccessRate < 0.80 && prev.agentSuccessRate >= 0.80 && curr.AgentCount > 0 {
		alerts = append(alerts, Alert{
			Level:   "warning",
			Title:   "Agent success rate dropped",
			Message: fmt.Sprintf("Success rate is %.0f%% (was %.0f%%)", curr.agentSuccessRate*100, prev.agentSuccessRate*100),
			Time:    now,
		})
	}

	return alerts
}

// compareInfo detects informational changes.
func compareInfo(prev, curr *WatchState) []Alert {
	var alerts []Alert
	now := time.Now()

	// New session completed.
	if curr.SessionCount > prev.SessionCount {
		newSessions := findNewSessions(prev, curr)
		for _, s := range newSessions {
			totalTools := 0
			for _, count := range s.ToolCounts {
				totalTools += count
			}
			alerts = append(alerts, Alert{
				Level:   "info",
				Title:   fmt.Sprintf("Session completed: %s", filepath.Base(s.ProjectPath)),
				Message: fmt.Sprintf("%dmin, %d commits, %d tool calls", s.DurationMinutes, s.GitCommits, totalTools),
				Time:    now,
			})
		}
	}

	// Friction type improved (frequency decreased by >20%).
	for frictionType, prevCount := range prev.FrictionCounts {
		currCount := curr.FrictionCounts[frictionType]
		if prevCount == 0 {
			continue
		}
		decrease := float64(prevCount-currCount) / float64(prevCount)
		if decrease > 0.20 && currCount < prevCount {
			alerts = append(alerts, Alert{
				Level:   "info",
				Title:   fmt.Sprintf("Friction improved: %s", frictionType),
				Message: fmt.Sprintf("Decreased from %d to %d (-%.0f%%)", prevCount, currCount, decrease*100),
				Time:    now,
			})
		}
	}

	// First session in a new project detected.
	if curr.SessionCount > prev.SessionCount {
		newSessions := findNewSessions(prev, curr)
		prevProjects := make(map[string]bool)
		for _, s := range prev.sessions {
			prevProjects[s.ProjectPath] = true
		}
		for _, s := range newSessions {
			if s.ProjectPath != "" && !prevProjects[s.ProjectPath] {
				alerts = append(alerts, Alert{
					Level:   "info",
					Title:   fmt.Sprintf("New project: %s", filepath.Base(s.ProjectPath)),
					Message: fmt.Sprintf("First session detected in %s", s.ProjectPath),
					Time:    now,
				})
			}
		}
	}

	// Stale pattern count improved (decreased).
	if curr.StalePatterns < prev.StalePatterns {
		alerts = append(alerts, Alert{
			Level:   "info",
			Title:   "Stale friction resolved",
			Message: fmt.Sprintf("Stale patterns decreased from %d to %d", prev.StalePatterns, curr.StalePatterns),
			Time:    now,
		})
	}

	return alerts
}

// findNewSessions returns sessions present in curr but not in prev,
// identified by session ID.
func findNewSessions(prev, curr *WatchState) []sessionInfo {
	prevIDs := make(map[string]bool, len(prev.sessions))
	for _, s := range prev.sessions {
		prevIDs[s.SessionID] = true
	}

	var newSessions []sessionInfo
	for _, s := range curr.sessions {
		if !prevIDs[s.SessionID] {
			newSessions = append(newSessions, sessionInfo{
				SessionID:         s.SessionID,
				ProjectPath:       s.ProjectPath,
				DurationMinutes:   s.DurationMinutes,
				GitCommits:        s.GitCommits,
				ToolCounts:        s.ToolCounts,
				UserInterruptions: s.UserInterruptions,
			})
		}
	}
	return newSessions
}

// sessionInfo is a lightweight summary of a session for alert generation.
type sessionInfo struct {
	SessionID         string
	ProjectPath       string
	DurationMinutes   int
	GitCommits        int
	ToolCounts        map[string]int
	UserInterruptions int
}
