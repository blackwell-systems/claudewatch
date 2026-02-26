package analyzer

import (
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// AnalyzeFriction aggregates friction patterns across all session facets.
// The threshold parameter (0.0-1.0) determines the minimum session frequency
// for a friction type to be considered "recurring" (e.g., 0.30 = 30%).
func AnalyzeFriction(facets []claude.SessionFacet, threshold float64) FrictionSummary {
	summary := FrictionSummary{
		FrictionByType:    make(map[string]int),
		FrictionByProject: make(map[string]int),
		TotalSessions:     len(facets),
	}

	if len(facets) == 0 {
		return summary
	}

	// Track which friction types appear in each session for frequency calculation.
	typeSessionCount := make(map[string]int)

	for _, facet := range facets {
		if len(facet.FrictionCounts) == 0 {
			continue
		}

		summary.SessionsWithFriction++

		for frictionType, count := range facet.FrictionCounts {
			summary.FrictionByType[frictionType] += count
			summary.TotalFrictionEvents += count
			typeSessionCount[frictionType]++
		}
	}

	// Identify recurring friction types: those appearing in >threshold of sessions.
	for frictionType, sessionCount := range typeSessionCount {
		frequency := float64(sessionCount) / float64(len(facets))
		if frequency > threshold {
			summary.RecurringFriction = append(summary.RecurringFriction, frictionType)
		}
	}

	// Sort recurring friction by frequency (highest first) for stable output.
	sort.Slice(summary.RecurringFriction, func(i, j int) bool {
		return typeSessionCount[summary.RecurringFriction[i]] > typeSessionCount[summary.RecurringFriction[j]]
	})

	return summary
}
