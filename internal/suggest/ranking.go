package suggest

import "sort"

// RankSuggestions sorts suggestions by ImpactScore in descending order.
func RankSuggestions(suggestions []Suggestion) []Suggestion {
	sorted := make([]Suggestion, len(suggestions))
	copy(sorted, suggestions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ImpactScore > sorted[j].ImpactScore
	})
	return sorted
}

// ComputeImpact calculates an impact score for a suggestion.
// Formula: (affectedSessions * frequency * timeSaved) / effort
//
// Parameters:
//   - affectedSessions: number of sessions affected by this issue
//   - frequency: how often the issue occurs (0.0-1.0)
//   - timeSaved: estimated minutes saved if the suggestion is implemented
//   - effort: estimated minutes of effort to implement the suggestion
//
// Returns 0 if effort is zero to avoid division by zero.
func ComputeImpact(affectedSessions int, frequency float64, timeSaved float64, effort float64) float64 {
	if effort <= 0 {
		return 0
	}
	return (float64(affectedSessions) * frequency * timeSaved) / effort
}
