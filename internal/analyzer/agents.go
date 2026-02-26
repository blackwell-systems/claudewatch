package analyzer

import (
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// AnalyzeAgents computes performance metrics for agent tasks.
func AnalyzeAgents(tasks []claude.AgentTask) AgentPerformance {
	perf := AgentPerformance{
		TotalAgents: len(tasks),
		ByType:      make(map[string]AgentTypeStats),
	}

	if len(tasks) == 0 {
		return perf
	}

	var totalDuration int64
	var totalTokens int
	var successCount, backgroundCount int

	// Group tasks by type for per-type stats.
	typeGroups := make(map[string][]claude.AgentTask)

	for _, task := range tasks {
		totalDuration += task.DurationMs
		totalTokens += task.TotalTokens

		if task.Status == "completed" {
			successCount++
		}
		if task.Background {
			backgroundCount++
		}

		typeGroups[task.AgentType] = append(typeGroups[task.AgentType], task)
	}

	n := float64(len(tasks))
	perf.SuccessRate = float64(successCount) / n
	perf.BackgroundRatio = float64(backgroundCount) / n
	perf.AvgDurationMs = float64(totalDuration) / n
	perf.AvgTokensPerAgent = float64(totalTokens) / n

	// Compute per-type stats.
	for agentType, typeTasks := range typeGroups {
		var typeSuccess int
		var typeDuration int64
		var typeTokens int

		for _, task := range typeTasks {
			typeDuration += task.DurationMs
			typeTokens += task.TotalTokens
			if task.Status == "completed" {
				typeSuccess++
			}
		}

		tn := float64(len(typeTasks))
		perf.ByType[agentType] = AgentTypeStats{
			Count:         len(typeTasks),
			SuccessRate:   float64(typeSuccess) / tn,
			AvgDurationMs: float64(typeDuration) / tn,
			AvgTokens:     float64(typeTokens) / tn,
		}
	}

	return perf
}
