package claude

import (
	"encoding/json"
)

// ContextPressureStats holds context window utilization data.
type ContextPressureStats struct {
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalTokens       int     `json:"total_tokens"`
	Compactions       int     `json:"compactions"`
	EstimatedUsage    float64 `json:"estimated_usage"` // 0.0-1.0
	Status            string  `json:"status"`          // "comfortable","filling","pressure","critical"
}

const contextWindowTokens = 200_000

// ParseLiveContextPressure reads the JSONL file at path and computes context
// window utilization from cumulative token usage and compaction events.
func ParseLiveContextPressure(path string) (*ContextPressureStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	stats := &ContextPressureStats{}

	var lastInputTokens int

	for _, entry := range entries {
		// Count compaction events.
		if entry.Type == "summary" {
			stats.Compactions++
			continue
		}

		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}

		var msg assistantMsgUsage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		stats.TotalInputTokens += msg.Usage.InputTokens
		stats.TotalOutputTokens += msg.Usage.OutputTokens

		if msg.Usage.InputTokens > 0 {
			lastInputTokens = msg.Usage.InputTokens
		}
	}

	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens

	// Use the most recent assistant message's input_tokens as the best proxy
	// for current context size (it represents the full context sent in the
	// last turn).
	if lastInputTokens > 0 {
		stats.EstimatedUsage = float64(lastInputTokens) / float64(contextWindowTokens)
	}

	switch {
	case stats.EstimatedUsage >= 0.9:
		stats.Status = "critical"
	case stats.EstimatedUsage >= 0.75:
		stats.Status = "pressure"
	case stats.EstimatedUsage >= 0.5:
		stats.Status = "filling"
	default:
		stats.Status = "comfortable"
	}

	return stats, nil
}
