package analyzer

import (
	"fmt"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// RegressionInput holds all data required to compute a regression status for a project.
type RegressionInput struct {
	Project        string
	Baseline       *store.ProjectBaseline
	RecentSessions []claude.SessionMeta
	Facets         []claude.SessionFacet
	Pricing        ModelPricing
	CacheRatio     CacheRatio
	Threshold      float64
}

// RegressionStatus is the result of a regression check for a project.
type RegressionStatus struct {
	Project              string  `json:"project"`
	HasBaseline          bool    `json:"has_baseline"`
	InsufficientData     bool    `json:"insufficient_data"`
	Regressed            bool    `json:"regressed"`
	FrictionRegressed    bool    `json:"friction_regressed"`
	CostRegressed        bool    `json:"cost_regressed"`
	CurrentFrictionRate  float64 `json:"current_friction_rate"`
	BaselineFrictionRate float64 `json:"baseline_friction_rate"`
	CurrentAvgCostUSD    float64 `json:"current_avg_cost_usd"`
	BaselineAvgCostUSD   float64 `json:"baseline_avg_cost_usd"`
	Threshold            float64 `json:"threshold"`
	Message              string  `json:"message"`
}

// ComputeRegressionStatus is a pure function that determines whether the recent
// sessions for a project represent a regression compared to the stored baseline.
// All data arrives via RegressionInput; no I/O or global state is accessed.
func ComputeRegressionStatus(input RegressionInput) RegressionStatus {
	// Guard: no baseline available.
	if input.Baseline == nil {
		return RegressionStatus{
			Project:     input.Project,
			HasBaseline: false,
			Message:     "no baseline available",
		}
	}

	// Guard: insufficient recent data.
	if len(input.RecentSessions) < 3 {
		return RegressionStatus{
			Project:          input.Project,
			HasBaseline:      true,
			InsufficientData: true,
			Message:          "insufficient data: fewer than 3 recent sessions",
		}
	}

	// Apply threshold default.
	threshold := input.Threshold
	if threshold <= 1 {
		threshold = 1.5
	}

	baseline := input.Baseline

	// Build facet index for fast friction lookup.
	facetsByID := buildFacetIndex(input.Facets)

	// Compute current friction rate: fraction of sessions with any friction.
	frictionSessions := 0
	for _, sess := range input.RecentSessions {
		if sessionFriction(sess, facetsByID) > 0 {
			frictionSessions++
		}
	}
	currentFrictionRate := float64(frictionSessions) / float64(len(input.RecentSessions))

	// Compute current average cost per session.
	var totalCost float64
	for _, sess := range input.RecentSessions {
		totalCost += EstimateSessionCost(sess, input.Pricing, input.CacheRatio)
	}
	currentAvgCost := totalCost / float64(len(input.RecentSessions))

	// Compare against baseline; skip comparison when baseline value is zero
	// to avoid spurious regressions.
	frictionRegressed := false
	if baseline.AvgFriction > 0 {
		frictionRegressed = currentFrictionRate > threshold*baseline.AvgFriction
	}

	costRegressed := false
	if baseline.AvgCostUSD > 0 {
		costRegressed = currentAvgCost > threshold*baseline.AvgCostUSD
	}

	regressed := frictionRegressed || costRegressed

	// Build message.
	var message string
	switch {
	case !regressed:
		message = "no regression detected"
	case frictionRegressed && costRegressed:
		message = fmt.Sprintf(
			"friction rate regressed (%.2f vs baseline %.2f) and cost regressed ($%.4f vs baseline $%.4f)",
			currentFrictionRate, baseline.AvgFriction,
			currentAvgCost, baseline.AvgCostUSD,
		)
	case frictionRegressed:
		message = fmt.Sprintf(
			"friction rate regressed (%.2f vs baseline %.2f, threshold %.1fx)",
			currentFrictionRate, baseline.AvgFriction, threshold,
		)
	default:
		message = fmt.Sprintf(
			"cost regressed ($%.4f vs baseline $%.4f, threshold %.1fx)",
			currentAvgCost, baseline.AvgCostUSD, threshold,
		)
	}

	return RegressionStatus{
		Project:              input.Project,
		HasBaseline:          true,
		InsufficientData:     false,
		Regressed:            regressed,
		FrictionRegressed:    frictionRegressed,
		CostRegressed:        costRegressed,
		CurrentFrictionRate:  currentFrictionRate,
		BaselineFrictionRate: baseline.AvgFriction,
		CurrentAvgCostUSD:    currentAvgCost,
		BaselineAvgCostUSD:   baseline.AvgCostUSD,
		Threshold:            threshold,
		Message:              message,
	}
}
