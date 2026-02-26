package analyzer

import (
	"math"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ModelPricing holds per-million-token pricing for a single model tier.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// DefaultPricing maps model tier names to their per-million-token pricing
// as of Feb 2026 for Claude models.
var DefaultPricing = map[string]ModelPricing{
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

// CostEstimate holds the computed cost breakdown from token usage data.
type CostEstimate struct {
	TotalCost      float64 `json:"total_cost"`
	InputCost      float64 `json:"input_cost"`
	OutputCost     float64 `json:"output_cost"`
	CacheReadCost  float64 `json:"cache_read_cost"`
	CacheWriteCost float64 `json:"cache_write_cost"`

	CostPerSession float64 `json:"cost_per_session"`
	CostPerCommit  float64 `json:"cost_per_commit"`
	CostPerDay     float64 `json:"cost_per_day"`

	// CacheSavings is the dollar amount saved by cache reads vs full input price.
	CacheSavings        float64 `json:"cache_savings"`
	CacheSavingsPercent float64 `json:"cache_savings_percent"`

	// ProjectCosts breaks down costs per project when session metadata is available.
	ProjectCosts []ProjectCost `json:"project_costs,omitempty"`
}

// ProjectCost holds per-project cost information.
type ProjectCost struct {
	ProjectName    string  `json:"project_name"`
	TotalCost      float64 `json:"total_cost"`
	Sessions       int     `json:"sessions"`
	CostPerSession float64 `json:"cost_per_session"`
}

// tokensToCost converts a token count to a dollar amount given a per-million rate.
func tokensToCost(tokens int64, perMillion float64) float64 {
	return float64(tokens) / 1_000_000.0 * perMillion
}

// EstimateCosts computes cost estimates from stats cache data.
// Model defaults to "sonnet" if not specified.
// totalSessions and totalCommits come from session metadata and are used
// for per-session and per-commit averages.
func EstimateCosts(stats claude.StatsCache, model string, totalSessions int, totalCommits int) CostEstimate {
	if model == "" {
		model = "sonnet"
	}
	model = strings.ToLower(model)

	pricing, ok := DefaultPricing[model]
	if !ok {
		pricing = DefaultPricing["sonnet"]
	}

	// Aggregate token counts across all models in the stats cache.
	var totalInput, totalOutput, totalCacheRead, totalCacheWrite int64
	for _, usage := range stats.ModelUsage {
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalCacheRead += usage.CacheReadInputTokens
		totalCacheWrite += usage.CacheCreationInputTokens
	}

	est := CostEstimate{
		InputCost:      tokensToCost(totalInput, pricing.InputPerMillion),
		OutputCost:     tokensToCost(totalOutput, pricing.OutputPerMillion),
		CacheReadCost:  tokensToCost(totalCacheRead, pricing.CacheReadPerMillion),
		CacheWriteCost: tokensToCost(totalCacheWrite, pricing.CacheWritePerMillion),
	}
	est.TotalCost = est.InputCost + est.OutputCost + est.CacheReadCost + est.CacheWriteCost

	// Cache savings: what cache reads would have cost at full input price minus
	// what they actually cost at the discounted cache read price.
	fullInputCostForCacheReads := tokensToCost(totalCacheRead, pricing.InputPerMillion)
	est.CacheSavings = fullInputCostForCacheReads - est.CacheReadCost

	// Savings percent: savings relative to what total would have been without caching.
	totalWithoutCacheSavings := est.TotalCost + est.CacheSavings
	if totalWithoutCacheSavings > 0 {
		est.CacheSavingsPercent = (est.CacheSavings / totalWithoutCacheSavings) * 100.0
	}

	// Per-session cost.
	if totalSessions > 0 {
		est.CostPerSession = est.TotalCost / float64(totalSessions)
	}

	// Per-commit cost (infinity if no commits).
	if totalCommits > 0 {
		est.CostPerCommit = est.TotalCost / float64(totalCommits)
	} else {
		est.CostPerCommit = math.Inf(1)
	}

	// Per-day cost based on daily activity entries.
	days := len(stats.DailyActivity)
	if days > 0 {
		est.CostPerDay = est.TotalCost / float64(days)
	}

	return est
}
