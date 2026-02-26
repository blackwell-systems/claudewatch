package analyzer

import (
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// satisfactionWeights maps satisfaction level strings to numeric weights.
var satisfactionWeights = map[string]float64{
	"satisfied":        1.0,
	"likely_satisfied": 0.75,
	"neutral":          0.5,
	"dissatisfied":     0.0,
}

// AnalyzeSatisfaction computes a weighted satisfaction score from session facets.
// The WeightedScore is 0-100 where 100 means all sessions were fully satisfied.
func AnalyzeSatisfaction(facets []claude.SessionFacet) SatisfactionScore {
	score := SatisfactionScore{
		SatisfactionCounts: make(map[string]int),
		OutcomeCounts:      make(map[string]int),
		HelpfulnessCounts:  make(map[string]int),
		TotalFacets:        len(facets),
	}

	if len(facets) == 0 {
		return score
	}

	var totalWeight float64
	var totalEntries int

	for _, facet := range facets {
		// Aggregate satisfaction counts across all facets.
		for level, count := range facet.UserSatisfactionCounts {
			score.SatisfactionCounts[level] += count

			weight, ok := satisfactionWeights[level]
			if !ok {
				weight = 0.5 // Default to neutral for unknown levels.
			}
			totalWeight += weight * float64(count)
			totalEntries += count
		}

		// Track outcome distribution.
		if facet.Outcome != "" {
			score.OutcomeCounts[facet.Outcome]++
		}

		// Track helpfulness distribution.
		if facet.ClaudeHelpfulness != "" {
			score.HelpfulnessCounts[facet.ClaudeHelpfulness]++
		}
	}

	if totalEntries > 0 {
		score.WeightedScore = (totalWeight / float64(totalEntries)) * 100.0
	}

	return score
}
