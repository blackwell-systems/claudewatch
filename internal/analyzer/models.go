package analyzer

import (
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ModelTier classifies a model name into a pricing tier.
type ModelTier string

const (
	TierOpus   ModelTier = "opus"
	TierSonnet ModelTier = "sonnet"
	TierHaiku  ModelTier = "haiku"
	TierOther  ModelTier = "other"
)

// ModelBreakdown holds per-model token and cost analysis.
type ModelBreakdown struct {
	ModelName    string    `json:"model_name"`
	Tier         ModelTier `json:"tier"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	CacheReads   int64     `json:"cache_reads"`
	CacheWrites  int64     `json:"cache_writes"`
	TotalTokens  int64     `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	CostPercent  float64   `json:"cost_percent"`
	TokenPercent float64   `json:"token_percent"`
}

// ModelAnalysis is the top-level result of model usage analysis.
type ModelAnalysis struct {
	// Models is the per-model breakdown sorted by cost descending.
	Models []ModelBreakdown `json:"models"`

	// TierCosts maps tier name to total cost for that tier.
	TierCosts map[ModelTier]float64 `json:"tier_costs"`

	// TierTokens maps tier name to total tokens for that tier.
	TierTokens map[ModelTier]int64 `json:"tier_tokens"`

	// DominantTier is the tier consuming the most cost.
	DominantTier ModelTier `json:"dominant_tier"`

	// OpusCostPercent is the percentage of total cost going to Opus-tier models.
	OpusCostPercent float64 `json:"opus_cost_percent"`

	// PotentialSavings is the estimated savings if all Opus usage were on Sonnet.
	PotentialSavings float64 `json:"potential_savings"`

	// DailyTrend shows the dominant model tier per day over time.
	DailyTrend []DailyModelMix `json:"daily_trend,omitempty"`

	// TotalCost is the sum across all models.
	TotalCost float64 `json:"total_cost"`

	// TotalTokens is the sum across all models.
	TotalTokens int64 `json:"total_tokens"`
}

// DailyModelMix shows model usage for a single day.
type DailyModelMix struct {
	Date         string            `json:"date"`
	TokensByTier map[ModelTier]int `json:"tokens_by_tier"`
	DominantTier ModelTier         `json:"dominant_tier"`
}

// TokenSummary provides a raw token breakdown for display.
type TokenSummary struct {
	TotalInput       int64   `json:"total_input"`
	TotalOutput      int64   `json:"total_output"`
	TotalCacheReads  int64   `json:"total_cache_reads"`
	TotalCacheWrites int64   `json:"total_cache_writes"`
	TotalTokens      int64   `json:"total_tokens"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
	InputOutputRatio float64 `json:"input_output_ratio"`

	// Per-session averages.
	AvgInputPerSession  int64 `json:"avg_input_per_session"`
	AvgOutputPerSession int64 `json:"avg_output_per_session"`
	AvgTotalPerSession  int64 `json:"avg_total_per_session"`
	TotalSessions       int   `json:"total_sessions"`
}

// ClassifyModelTier maps a model name to its pricing tier.
func ClassifyModelTier(modelName string) ModelTier {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "opus"):
		return TierOpus
	case strings.Contains(lower, "sonnet"):
		return TierSonnet
	case strings.Contains(lower, "haiku"):
		return TierHaiku
	default:
		return TierOther
	}
}

// AnalyzeModels computes per-model cost and token breakdowns from stats cache data.
func AnalyzeModels(stats *claude.StatsCache) ModelAnalysis {
	result := ModelAnalysis{
		TierCosts:  make(map[ModelTier]float64),
		TierTokens: make(map[ModelTier]int64),
	}

	if stats == nil || len(stats.ModelUsage) == 0 {
		return result
	}

	// Build per-model breakdowns.
	for name, usage := range stats.ModelUsage {
		tier := ClassifyModelTier(name)
		total := usage.InputTokens + usage.OutputTokens
		mb := ModelBreakdown{
			ModelName:    name,
			Tier:         tier,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			CacheReads:   usage.CacheReadInputTokens,
			CacheWrites:  usage.CacheCreationInputTokens,
			TotalTokens:  total,
			CostUSD:      usage.CostUSD,
		}
		result.Models = append(result.Models, mb)
		result.TotalCost += usage.CostUSD
		result.TotalTokens += total
		result.TierCosts[tier] += usage.CostUSD
		result.TierTokens[tier] += total
	}

	// Compute percentages.
	for i := range result.Models {
		if result.TotalCost > 0 {
			result.Models[i].CostPercent = result.Models[i].CostUSD / result.TotalCost * 100
		}
		if result.TotalTokens > 0 {
			result.Models[i].TokenPercent = float64(result.Models[i].TotalTokens) / float64(result.TotalTokens) * 100
		}
	}

	// Sort by cost descending.
	sort.Slice(result.Models, func(i, j int) bool {
		return result.Models[i].CostUSD > result.Models[j].CostUSD
	})

	// Dominant tier by cost.
	maxCost := 0.0
	for tier, cost := range result.TierCosts {
		if cost > maxCost {
			maxCost = cost
			result.DominantTier = tier
		}
	}

	// Opus cost percentage.
	if result.TotalCost > 0 {
		result.OpusCostPercent = result.TierCosts[TierOpus] / result.TotalCost * 100
	}

	// Potential savings: what Opus usage would cost at Sonnet rates.
	// This is a rough estimate — we recalculate Opus model tokens at Sonnet pricing.
	sonnetPricing := DefaultPricing["sonnet"]
	for _, mb := range result.Models {
		if mb.Tier == TierOpus {
			sonnetEquiv := tokensToCost(mb.InputTokens, sonnetPricing.InputPerMillion) +
				tokensToCost(mb.OutputTokens, sonnetPricing.OutputPerMillion) +
				tokensToCost(mb.CacheReads, sonnetPricing.CacheReadPerMillion) +
				tokensToCost(mb.CacheWrites, sonnetPricing.CacheWritePerMillion)
			result.PotentialSavings += mb.CostUSD - sonnetEquiv
		}
	}

	// Daily model mix trend.
	if len(stats.DailyModelTokens) > 0 {
		for _, day := range stats.DailyModelTokens {
			mix := DailyModelMix{
				Date:         day.Date,
				TokensByTier: make(map[ModelTier]int),
			}
			maxTokens := 0
			for model, tokens := range day.TokensByModel {
				tier := ClassifyModelTier(model)
				mix.TokensByTier[tier] += tokens
				if mix.TokensByTier[tier] > maxTokens {
					maxTokens = mix.TokensByTier[tier]
					mix.DominantTier = tier
				}
			}
			result.DailyTrend = append(result.DailyTrend, mix)
		}
	}

	return result
}

// AnalyzeTokens computes a raw token summary from stats cache and session data.
func AnalyzeTokens(stats *claude.StatsCache, sessions []claude.SessionMeta) TokenSummary {
	var ts TokenSummary

	// Aggregate from stats cache (authoritative for totals).
	if stats != nil {
		for _, usage := range stats.ModelUsage {
			ts.TotalInput += usage.InputTokens
			ts.TotalOutput += usage.OutputTokens
			ts.TotalCacheReads += usage.CacheReadInputTokens
			ts.TotalCacheWrites += usage.CacheCreationInputTokens
		}
	}

	ts.TotalTokens = ts.TotalInput + ts.TotalOutput

	// Cache hit rate: cache reads / (cache reads + uncached input).
	cacheEligible := ts.TotalCacheReads + ts.TotalInput
	if cacheEligible > 0 {
		ts.CacheHitRate = float64(ts.TotalCacheReads) / float64(cacheEligible) * 100
		if ts.CacheHitRate > 100 {
			ts.CacheHitRate = 100
		}
	}

	// Input/output ratio.
	if ts.TotalOutput > 0 {
		ts.InputOutputRatio = float64(ts.TotalInput) / float64(ts.TotalOutput)
	}

	// Per-session averages from session meta (more granular than stats cache).
	ts.TotalSessions = len(sessions)
	if len(sessions) > 0 {
		var totalIn, totalOut int64
		for _, s := range sessions {
			totalIn += int64(s.InputTokens)
			totalOut += int64(s.OutputTokens)
		}
		n := int64(len(sessions))
		ts.AvgInputPerSession = totalIn / n
		ts.AvgOutputPerSession = totalOut / n
		ts.AvgTotalPerSession = (totalIn + totalOut) / n
	}

	return ts
}
