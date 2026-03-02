package analyzer

import (
	"fmt"
	"math"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// BaselineInput is the data required to compute a project baseline.
type BaselineInput struct {
	Project    string
	Sessions   []claude.SessionMeta
	Facets     []claude.SessionFacet
	SAWIDs     map[string]bool // set of SAW session IDs
	Pricing    ModelPricing
	CacheRatio CacheRatio
}

// ComputeProjectBaseline computes the statistical baseline for a project
// using all historical sessions. Requires at least 3 sessions; returns an
// error if fewer sessions are available.
func ComputeProjectBaseline(input BaselineInput) (store.ProjectBaseline, error) {
	if len(input.Sessions) < 3 {
		return store.ProjectBaseline{}, fmt.Errorf(
			"insufficient session history for project %q: need at least 3 sessions, got %d",
			input.Project, len(input.Sessions),
		)
	}

	facetsByID := buildFacetIndex(input.Facets)

	n := float64(len(input.Sessions))

	// Compute per-session costs and frictions.
	costs := make([]float64, len(input.Sessions))
	frictions := make([]float64, len(input.Sessions))
	var totalCommits float64
	sawCount := 0

	for i, sess := range input.Sessions {
		costs[i] = EstimateSessionCost(sess, input.Pricing, input.CacheRatio)
		frictions[i] = float64(sessionFriction(sess, facetsByID))
		totalCommits += float64(sess.GitCommits)
		if input.SAWIDs[sess.SessionID] {
			sawCount++
		}
	}

	// Compute means.
	avgCost := mean(costs)
	avgFriction := mean(frictions)

	// Compute population stddevs.
	stddevCost := populationStddev(costs, avgCost)
	stddevFriction := populationStddev(frictions, avgFriction)

	return store.ProjectBaseline{
		Project:        input.Project,
		ComputedAt:     time.Now().UTC().Format(time.RFC3339),
		SessionCount:   len(input.Sessions),
		AvgCostUSD:     avgCost,
		StddevCostUSD:  stddevCost,
		AvgFriction:    avgFriction,
		StddevFriction: stddevFriction,
		AvgCommits:     totalCommits / n,
		SAWSessionFrac: float64(sawCount) / n,
	}, nil
}

// DetectAnomalies scans sessions against a stored baseline and returns
// sessions that deviate by more than threshold standard deviations.
// threshold defaults to 2.0 if <= 0.
func DetectAnomalies(
	sessions []claude.SessionMeta,
	facets []claude.SessionFacet,
	baseline store.ProjectBaseline,
	pricing ModelPricing,
	cacheRatio CacheRatio,
	threshold float64,
) []store.AnomalyResult {
	if threshold <= 0 {
		threshold = 2.0
	}

	facetsByID := buildFacetIndex(facets)
	var results []store.AnomalyResult

	for _, sess := range sessions {
		cost := EstimateSessionCost(sess, pricing, cacheRatio)
		friction := sessionFriction(sess, facetsByID)

		costZ := zScore(cost, baseline.AvgCostUSD, baseline.StddevCostUSD)
		frictionZ := zScore(float64(friction), baseline.AvgFriction, baseline.StddevFriction)

		if math.Abs(costZ) <= threshold && math.Abs(frictionZ) <= threshold {
			continue
		}

		severity := "warning"
		if math.Abs(costZ) >= 3 || math.Abs(frictionZ) >= 3 {
			severity = "critical"
		}

		reason := buildAnomalyReason(costZ, frictionZ, threshold)

		results = append(results, store.AnomalyResult{
			SessionID:      sess.SessionID,
			Project:        baseline.Project,
			StartTime:      sess.StartTime,
			CostUSD:        cost,
			Friction:       friction,
			CostZScore:     costZ,
			FrictionZScore: frictionZ,
			Severity:       severity,
			Reason:         reason,
		})
	}

	return results
}

// mean returns the arithmetic mean of a slice of float64 values.
// Returns 0 if the slice is empty.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// populationStddev returns the population standard deviation:
// sqrt(sum((x - mean)^2) / n).
// Returns 0 if the slice is empty.
func populationStddev(values []float64, avg float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sumSq float64
	for _, v := range values {
		diff := v - avg
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(values)))
}

// zScore computes the standard score: (value - mean) / stddev.
// Returns 0 if stddev is 0 (no variance in the data).
func zScore(value, avg, stddev float64) float64 {
	if stddev == 0 {
		return 0
	}
	return (value - avg) / stddev
}

// buildAnomalyReason constructs a human-readable reason string for an anomaly.
func buildAnomalyReason(costZ, frictionZ, threshold float64) string {
	costAnomalous := math.Abs(costZ) > threshold
	frictionAnomalous := math.Abs(frictionZ) > threshold

	switch {
	case costAnomalous && frictionAnomalous:
		return fmt.Sprintf("high cost (z=%.2f) and high friction (z=%.2f)", costZ, frictionZ)
	case costAnomalous:
		return fmt.Sprintf("high cost deviation (z=%.2f)", costZ)
	default:
		return fmt.Sprintf("high friction deviation (z=%.2f)", frictionZ)
	}
}
