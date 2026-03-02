package claude

import (
	"encoding/json"
	"time"
)

// CostPricing holds per-million-token pricing for input and output tokens.
type CostPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// LiveCostVelocityStats holds cost velocity data within a rolling time window.
type LiveCostVelocityStats struct {
	WindowMinutes float64 `json:"window_minutes"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	CostPerMinute float64 `json:"cost_per_minute"`
	Status        string  `json:"status"` // "efficient","normal","burning"
}

// ParseLiveCostVelocity reads the JSONL file at path and computes cost velocity
// within the last windowMinutes. Uses assistant entry timestamps and per-turn
// usage fields to calculate cost based on the provided pricing.
func ParseLiveCostVelocity(path string, windowMinutes float64, pricing CostPricing) (*LiveCostVelocityStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-time.Duration(windowMinutes * float64(time.Minute)))
	stats := &LiveCostVelocityStats{WindowMinutes: windowMinutes}

	var inputTokens, outputTokens int

	for _, entry := range entries {
		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}
		ts := ParseTimestamp(entry.Timestamp)
		if ts.IsZero() || ts.Before(cutoff) {
			continue
		}
		var msg assistantMsgUsage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}
		inputTokens += msg.Usage.InputTokens
		outputTokens += msg.Usage.OutputTokens
	}

	stats.WindowCostUSD = (float64(inputTokens)/1_000_000)*pricing.InputPerMillion +
		(float64(outputTokens)/1_000_000)*pricing.OutputPerMillion

	if windowMinutes > 0 {
		stats.CostPerMinute = stats.WindowCostUSD / windowMinutes
	}

	switch {
	case stats.CostPerMinute < 0.05:
		stats.Status = "efficient"
	case stats.CostPerMinute < 0.20:
		stats.Status = "normal"
	default:
		stats.Status = "burning"
	}

	return stats, nil
}
