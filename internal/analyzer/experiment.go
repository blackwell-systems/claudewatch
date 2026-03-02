package analyzer

import (
	"fmt"
	"math"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// VariantStats holds aggregate outcome metrics for one experiment variant.
type VariantStats struct {
	Variant      string  `json:"variant"`
	SessionCount int     `json:"session_count"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	AvgFriction  float64 `json:"avg_friction"`
	AvgCommits   float64 `json:"avg_commits"`
}

// ExperimentReport is the top-level result of an A/B experiment analysis.
type ExperimentReport struct {
	ExperimentID int64        `json:"experiment_id"`
	Project      string       `json:"project"`
	A            VariantStats `json:"variant_a"`
	B            VariantStats `json:"variant_b"`
	Winner       string       `json:"winner"`     // "a", "b", or "inconclusive"
	Confidence   string       `json:"confidence"` // "high", "low", "inconclusive"
	Summary      string       `json:"summary"`
}

// AnalyzeExperiment computes per-variant outcome metrics for an A/B experiment
// and determines a winner based on cost, friction, and commit metrics.
func AnalyzeExperiment(
	exp store.Experiment,
	sessions []claude.SessionMeta,
	facets []claude.SessionFacet,
	assignments map[string]string, // sessionID → "a" or "b"
	pricing ModelPricing,
	ratio CacheRatio,
) ExperimentReport {
	// Build facet index for fast lookup.
	facetIndex := make(map[string]claude.SessionFacet, len(facets))
	for _, f := range facets {
		facetIndex[f.SessionID] = f
	}

	// Partition sessions into variants.
	var variantA, variantB []claude.SessionMeta
	for _, s := range sessions {
		v, ok := assignments[s.SessionID]
		if !ok {
			continue
		}
		switch v {
		case "a":
			variantA = append(variantA, s)
		case "b":
			variantB = append(variantB, s)
		}
	}

	statsA := computeVariantStats("a", variantA, facetIndex, pricing, ratio)
	statsB := computeVariantStats("b", variantB, facetIndex, pricing, ratio)

	winner, confidence := determineWinner(statsA, statsB)

	summary := fmt.Sprintf(
		"Variant A: %d sessions, avg $%.2f, friction %.1f, commits %.1f. "+
			"Variant B: %d sessions, avg $%.2f, friction %.1f, commits %.1f. "+
			"Winner: %s",
		statsA.SessionCount, statsA.AvgCostUSD, statsA.AvgFriction, statsA.AvgCommits,
		statsB.SessionCount, statsB.AvgCostUSD, statsB.AvgFriction, statsB.AvgCommits,
		winner,
	)

	return ExperimentReport{
		ExperimentID: exp.ID,
		Project:      exp.Project,
		A:            statsA,
		B:            statsB,
		Winner:       winner,
		Confidence:   confidence,
		Summary:      summary,
	}
}

// computeVariantStats computes aggregate metrics for a single variant's sessions.
func computeVariantStats(
	variant string,
	sessions []claude.SessionMeta,
	facetIndex map[string]claude.SessionFacet,
	pricing ModelPricing,
	ratio CacheRatio,
) VariantStats {
	stats := VariantStats{Variant: variant, SessionCount: len(sessions)}
	if len(sessions) == 0 {
		return stats
	}

	costs := make([]float64, len(sessions))
	frictions := make([]float64, len(sessions))
	commits := make([]float64, len(sessions))

	for i, s := range sessions {
		costs[i] = EstimateSessionCost(s, pricing, ratio)

		var frictionTotal int
		if f, ok := facetIndex[s.SessionID]; ok {
			for _, count := range f.FrictionCounts {
				frictionTotal += count
			}
		}
		frictions[i] = float64(frictionTotal)
		commits[i] = float64(s.GitCommits)
	}

	stats.AvgCostUSD = mean(costs)
	stats.AvgFriction = mean(frictions)
	stats.AvgCommits = mean(commits)
	return stats
}

// determineWinner compares two variants and returns the winner and confidence.
func determineWinner(a, b VariantStats) (winner, confidence string) {
	if a.SessionCount < 5 || b.SessionCount < 5 {
		return "inconclusive", "inconclusive"
	}

	highConfidence := a.SessionCount >= 10 && b.SessionCount >= 10

	// Compare by cost first.
	maxCost := math.Max(a.AvgCostUSD, b.AvgCostUSD)
	if maxCost > 0 && math.Abs(a.AvgCostUSD-b.AvgCostUSD) > 0.1*maxCost {
		conf := "low"
		if highConfidence {
			conf = "high"
		}
		if a.AvgCostUSD < b.AvgCostUSD {
			return "a", conf
		}
		return "b", conf
	}

	// Cost within 10% — compare by friction.
	maxFriction := math.Max(a.AvgFriction, b.AvgFriction)
	if maxFriction > 0 && math.Abs(a.AvgFriction-b.AvgFriction) > 0.1*maxFriction {
		conf := "low"
		if highConfidence {
			conf = "high"
		}
		if a.AvgFriction < b.AvgFriction {
			return "a", conf
		}
		return "b", conf
	}

	// Both within threshold.
	return "inconclusive", "low"
}
