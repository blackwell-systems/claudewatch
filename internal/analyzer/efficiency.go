package analyzer

import (
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// AnalyzeEfficiency computes tool usage efficiency metrics from session data.
func AnalyzeEfficiency(sessions []claude.SessionMeta) EfficiencyMetrics {
	metrics := EfficiencyMetrics{
		ErrorCategoryTotals: make(map[string]int),
		ToolUsageTotals:     make(map[string]int),
		TotalSessions:       len(sessions),
	}

	if len(sessions) == 0 {
		return metrics
	}

	var totalErrors, totalInterruptions, totalTokens int

	for _, s := range sessions {
		totalErrors += s.ToolErrors
		totalInterruptions += s.UserInterruptions
		totalTokens += s.InputTokens + s.OutputTokens

		// Aggregate error categories.
		for category, count := range s.ToolErrorCategories {
			metrics.ErrorCategoryTotals[category] += count
		}

		// Aggregate tool usage.
		for tool, count := range s.ToolCounts {
			metrics.ToolUsageTotals[tool] += count
		}

		// Track feature adoption.
		if s.UsesTaskAgent {
			metrics.FeatureAdoption.TaskAgentSessions++
		}
		if s.UsesMCP {
			metrics.FeatureAdoption.MCPSessions++
		}
		if s.UsesWebSearch {
			metrics.FeatureAdoption.WebSearchSessions++
		}
		if s.UsesWebFetch {
			metrics.FeatureAdoption.WebFetchSessions++
		}
	}

	n := float64(len(sessions))
	metrics.AvgToolErrorsPerSession = float64(totalErrors) / n
	metrics.AvgInterruptionsPerSession = float64(totalInterruptions) / n
	metrics.AvgTokensPerSession = float64(totalTokens) / n
	metrics.FeatureAdoption.TotalSessions = len(sessions)

	return metrics
}
