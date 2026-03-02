package analyzer

import (
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// sessionFriction returns the total friction count for a session.
// It sums all values in the facet's FrictionCounts map, or falls back to
// sess.ToolErrors if no facet is found for the session.
func sessionFriction(sess claude.SessionMeta, facetsByID map[string]claude.SessionFacet) int {
	facet, ok := facetsByID[sess.SessionID]
	if !ok {
		return sess.ToolErrors
	}
	total := 0
	for _, v := range facet.FrictionCounts {
		total += v
	}
	return total
}

// buildFacetIndex builds a map from session ID to SessionFacet for fast lookup.
func buildFacetIndex(facets []claude.SessionFacet) map[string]claude.SessionFacet {
	m := make(map[string]claude.SessionFacet, len(facets))
	for _, f := range facets {
		m[f.SessionID] = f
	}
	return m
}

// computeComparisonGroup aggregates stats for a slice of SessionComparison entries.
func computeComparisonGroup(sessions []SessionComparison) ComparisonGroup {
	if len(sessions) == 0 {
		return ComparisonGroup{}
	}

	var totalCost float64
	var totalCommits int
	var totalFriction int

	for _, s := range sessions {
		totalCost += s.CostUSD
		totalCommits += s.GitCommits
		totalFriction += s.Friction
	}

	n := float64(len(sessions))
	g := ComparisonGroup{
		Count:       len(sessions),
		AvgCostUSD:  totalCost / n,
		AvgCommits:  float64(totalCommits) / n,
		AvgFriction: float64(totalFriction) / n,
	}

	if totalCommits > 0 {
		g.CostPerCommit = totalCost / float64(totalCommits)
	}

	return g
}

// CompareSAWVsSequential computes a ComparisonReport for a given project.
//
// sessions: all SessionMeta for the project (pre-filtered by project name).
// facets: all SessionFacet records (filtered internally by session ID).
// sawSessionIDs: map of sessionID -> wave count for SAW sessions.
// sawAgentCounts: map of sessionID -> total agent count for SAW sessions.
// pricing, cacheRatio: used for per-session cost estimation.
// includeSessions: if true, populates the Sessions field of the report.
func CompareSAWVsSequential(
	project string,
	sessions []claude.SessionMeta,
	facets []claude.SessionFacet,
	sawSessionIDs map[string]int,
	sawAgentCounts map[string]int,
	pricing ModelPricing,
	cacheRatio CacheRatio,
	includeSessions bool,
) ComparisonReport {
	facetsByID := buildFacetIndex(facets)

	var sawSessions []SessionComparison
	var seqSessions []SessionComparison

	for _, sess := range sessions {
		cost := EstimateSessionCost(sess, pricing, cacheRatio)
		friction := sessionFriction(sess, facetsByID)

		waveCount := sawSessionIDs[sess.SessionID]
		agentCount := sawAgentCounts[sess.SessionID]
		isSAW := waveCount > 0

		sc := SessionComparison{
			SessionID:  sess.SessionID,
			Project:    project,
			StartTime:  sess.StartTime,
			IsSAW:      isSAW,
			WaveCount:  waveCount,
			AgentCount: agentCount,
			CostUSD:    cost,
			GitCommits: sess.GitCommits,
			Friction:   friction,
		}

		if isSAW {
			sawSessions = append(sawSessions, sc)
		} else {
			seqSessions = append(seqSessions, sc)
		}
	}

	report := ComparisonReport{
		Project:    project,
		SAW:        computeComparisonGroup(sawSessions),
		Sequential: computeComparisonGroup(seqSessions),
	}

	if includeSessions {
		all := make([]SessionComparison, 0, len(sawSessions)+len(seqSessions))
		all = append(all, sawSessions...)
		all = append(all, seqSessions...)
		// Sort by start time descending.
		sort.Slice(all, func(i, j int) bool {
			return all[i].StartTime > all[j].StartTime
		})
		report.Sessions = all
	}

	return report
}
