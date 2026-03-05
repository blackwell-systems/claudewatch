package claude

import (
	"encoding/json"
	"strings"
	"time"
)

// CostPricing holds per-million-token pricing for input, output, and cache tokens.
type CostPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// ModelPricingMap maps model tier names (opus, sonnet, haiku) to their pricing.
// Used by per-model cost calculations to price each turn at the correct rate.
var ModelPricingMap = map[string]CostPricing{
	"opus": {
		InputPerMillion:      15.0,
		OutputPerMillion:     75.0,
		CacheReadPerMillion:  1.5,
		CacheWritePerMillion: 18.75,
	},
	"sonnet": {
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheReadPerMillion:  0.3,
		CacheWritePerMillion: 3.75,
	},
	"haiku": {
		InputPerMillion:      0.25,
		OutputPerMillion:     1.25,
		CacheReadPerMillion:  0.03,
		CacheWritePerMillion: 0.3,
	},
}

// PricingForModel returns the CostPricing for the given model name string.
// Classifies by checking for "opus", "sonnet", or "haiku" substrings.
// Defaults to sonnet pricing for unknown models.
func PricingForModel(modelName string) CostPricing {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "opus"):
		return ModelPricingMap["opus"]
	case strings.Contains(lower, "haiku"):
		return ModelPricingMap["haiku"]
	default:
		return ModelPricingMap["sonnet"]
	}
}

// computeTurnCost calculates the cost of a single assistant turn given its
// token usage and pricing.
func computeTurnCost(usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}, pricing CostPricing) float64 {
	return (float64(usage.InputTokens)/1_000_000)*pricing.InputPerMillion +
		(float64(usage.OutputTokens)/1_000_000)*pricing.OutputPerMillion +
		(float64(usage.CacheReadInputTokens)/1_000_000)*pricing.CacheReadPerMillion +
		(float64(usage.CacheCreationInputTokens)/1_000_000)*pricing.CacheWritePerMillion
}

// LiveCostVelocityStats holds cost velocity data within a rolling time window.
type LiveCostVelocityStats struct {
	WindowMinutes float64 `json:"window_minutes"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	CostPerMinute float64 `json:"cost_per_minute"`
	Status        string  `json:"status"` // "efficient","normal","burning"
}

// ParseLiveCostVelocity reads the JSONL file at path and computes cost velocity
// within the last windowMinutes. Uses per-model pricing: each assistant turn is
// priced according to the model that produced it. The pricing parameter is used
// as a fallback when the model field is absent from a turn.
func ParseLiveCostVelocity(path string, windowMinutes float64, pricing CostPricing) (*LiveCostVelocityStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-time.Duration(windowMinutes * float64(time.Minute)))
	stats := &LiveCostVelocityStats{WindowMinutes: windowMinutes}

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

		// Use per-model pricing when model is available, fallback otherwise.
		turnPricing := pricing
		if msg.Model != "" {
			turnPricing = PricingForModel(msg.Model)
		}
		stats.WindowCostUSD += computeTurnCost(msg.Usage, turnPricing)
	}

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
