package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// StalePatternsResult holds the result of analyzing stale friction patterns.
type StalePatternsResult struct {
	Patterns         []StalePattern `json:"patterns"`
	TotalSessions    int            `json:"total_sessions"`
	WindowSessions   int            `json:"window_sessions"`
	Threshold        float64        `json:"threshold"`
	ClaudeMDLookback int            `json:"claude_md_lookback_sessions"`
}

// StalePattern represents a recurring friction type that may be unaddressed.
type StalePattern struct {
	FrictionType    string  `json:"friction_type"`
	RecurrenceRate  float64 `json:"recurrence_rate"`
	SessionCount    int     `json:"session_count"`
	LastClaudeMDAge int     `json:"last_claude_md_age"`
	IsStale         bool    `json:"is_stale"`
}

// handleGetStalePatterns analyzes recent sessions to find recurring friction patterns
// that haven't been addressed by a CLAUDE.md update.
func (s *Server) handleGetStalePatterns(args json.RawMessage) (any, error) {
	// Parse optional args.
	threshold := 0.3
	lookback := 10

	if len(args) > 0 && string(args) != "null" {
		var params struct {
			Threshold *float64 `json:"threshold"`
			Lookback  *int     `json:"lookback"`
		}
		if err := json.Unmarshal(args, &params); err == nil {
			if params.Threshold != nil {
				threshold = *params.Threshold
			}
			if params.Lookback != nil {
				lookback = *params.Lookback
			}
		}
	}

	if lookback <= 0 {
		lookback = 10
	}

	// Load all sessions (non-fatal).
	sessions, _ := claude.ParseAllSessionMeta(s.claudeHome)
	totalSessions := len(sessions)

	// Load all facets (non-fatal).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

	// Index facets by session ID.
	facetMap := make(map[string]*claude.SessionFacet, len(facets))
	for i := range facets {
		facetMap[facets[i].SessionID] = &facets[i]
	}

	// Sort sessions descending by StartTime.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime > sessions[j].StartTime
	})

	// Take most recent `lookback` sessions as the window.
	window := sessions
	if lookback < len(sessions) {
		window = sessions[:lookback]
	}

	if len(window) == 0 {
		return StalePatternsResult{
			Patterns:         []StalePattern{},
			TotalSessions:    totalSessions,
			WindowSessions:   0,
			Threshold:        threshold,
			ClaudeMDLookback: lookback,
		}, nil
	}

	// Find the oldest session's StartTime in window.
	oldestStartTime := window[len(window)-1].StartTime
	oldestTime, err := time.Parse(time.RFC3339, oldestStartTime)
	if err != nil {
		// Try without timezone suffix.
		oldestTime, _ = time.Parse("2006-01-02T15:04:05", oldestStartTime)
	}

	// Collect unique project paths in the window.
	projectPaths := make(map[string]struct{})
	for _, sess := range window {
		if sess.ProjectPath != "" {
			projectPaths[sess.ProjectPath] = struct{}{}
		}
	}

	// For each project, check if CLAUDE.md was modified during the window.
	// A project is "addressed" if CLAUDE.md exists and was modified after the oldest window session.
	addressedProjects := make(map[string]bool)
	for projectPath := range projectPaths {
		claudeMDPath := filepath.Join(projectPath, "CLAUDE.md")
		info, err := os.Stat(claudeMDPath)
		if err != nil {
			// CLAUDE.md doesn't exist — not addressed.
			addressedProjects[projectPath] = false
			continue
		}
		// CLAUDE.md was modified after the oldest window session start → addressed.
		addressedProjects[projectPath] = info.ModTime().After(oldestTime)
	}

	// For each window session, collect friction types present.
	// frictionSessionCount[frictionType] = number of sessions having that friction type.
	frictionSessionCount := make(map[string]int)
	for _, sess := range window {
		facet, ok := facetMap[sess.SessionID]
		if !ok {
			continue
		}
		// Track which friction types this session has (deduplicated per session).
		seen := make(map[string]bool)
		for frictionType, count := range facet.FrictionCounts {
			if count > 0 && !seen[frictionType] {
				seen[frictionType] = true
				frictionSessionCount[frictionType]++
			}
		}
	}

	// Build StalePattern list.
	patterns := make([]StalePattern, 0, len(frictionSessionCount))
	windowLen := len(window)

	for frictionType, count := range frictionSessionCount {
		recurrenceRate := float64(count) / float64(windowLen)

		// Determine if any contributing project is unaddressed.
		anyUnaddressed := false
		for projectPath, addressed := range addressedProjects {
			_ = projectPath
			if !addressed {
				anyUnaddressed = true
				break
			}
		}

		isStale := recurrenceRate > threshold && anyUnaddressed

		patterns = append(patterns, StalePattern{
			FrictionType:    frictionType,
			RecurrenceRate:  recurrenceRate,
			SessionCount:    count,
			LastClaudeMDAge: 0, // 0 if CLAUDE.md doesn't exist or was never changed during window
			IsStale:         isStale,
		})
	}

	// Sort descending by RecurrenceRate.
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].RecurrenceRate != patterns[j].RecurrenceRate {
			return patterns[i].RecurrenceRate > patterns[j].RecurrenceRate
		}
		return patterns[i].FrictionType < patterns[j].FrictionType
	})

	return StalePatternsResult{
		Patterns:         patterns,
		TotalSessions:    totalSessions,
		WindowSessions:   windowLen,
		Threshold:        threshold,
		ClaudeMDLookback: lookback,
	}, nil
}
