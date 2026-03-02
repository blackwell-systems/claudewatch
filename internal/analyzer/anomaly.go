package analyzer

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// BaselineInput is the data required to compute a project baseline.
type BaselineInput struct {
	Project     string
	Sessions    []claude.SessionMeta
	Facets      []claude.SessionFacet
	SAWIDs      map[string]bool // set of SAW session IDs
	Pricing     ModelPricing
	CacheRatio  CacheRatio
	DecayFactor float64 // EMA decay per session step (0 < decay <= 1); 0 or >1 means equal weights. Use 0.9 for EMA.
}

// ComputeProjectBaseline computes the statistical baseline for a project
// using all historical sessions with exponential decay weighting so that
// recent sessions have more influence than older ones. Requires at least 3
// sessions; returns an error if fewer sessions are available.
func ComputeProjectBaseline(input BaselineInput) (store.ProjectBaseline, error) {
	if len(input.Sessions) < 3 {
		return store.ProjectBaseline{}, fmt.Errorf(
			"insufficient session history for project %q: need at least 3 sessions, got %d",
			input.Project, len(input.Sessions),
		)
	}

	decay := input.DecayFactor
	if decay <= 0 || decay > 1 {
		// Unset or invalid: fall back to equal weights (decay=1 → all weights are 1).
		decay = 1.0
	}

	// Sort sessions oldest-first so EMA weights increase toward the present.
	sorted := make([]claude.SessionMeta, len(input.Sessions))
	copy(sorted, input.Sessions)
	sortSessionsByTime(sorted)

	facetsByID := buildFacetIndex(input.Facets)
	n := len(sorted)

	costs := make([]float64, n)
	frictions := make([]float64, n)
	commits := make([]float64, n)
	sawFlags := make([]float64, n)

	for i, sess := range sorted {
		costs[i] = EstimateSessionCost(sess, input.Pricing, input.CacheRatio)
		frictions[i] = float64(sessionFriction(sess, facetsByID))
		commits[i] = float64(sess.GitCommits)
		if input.SAWIDs[sess.SessionID] {
			sawFlags[i] = 1.0
		}
	}

	// EMA weights: oldest session gets decay^(n-1), newest gets decay^0 = 1.
	weights := emaWeights(n, decay)

	avgCost := weightedMean(costs, weights)
	avgFriction := weightedMean(frictions, weights)

	return store.ProjectBaseline{
		Project:        input.Project,
		ComputedAt:     time.Now().UTC().Format(time.RFC3339),
		SessionCount:   n,
		AvgCostUSD:     avgCost,
		StddevCostUSD:  weightedStddev(costs, weights, avgCost),
		AvgFriction:    avgFriction,
		StddevFriction: weightedStddev(frictions, weights, avgFriction),
		AvgCommits:     weightedMean(commits, weights),
		SAWSessionFrac: weightedMean(sawFlags, weights),
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

// emaWeights returns exponential decay weights for n sessions ordered
// oldest-first. The newest session (index n-1) gets weight 1; each older
// session is scaled by decay, so index i gets decay^(n-1-i).
func emaWeights(n int, decay float64) []float64 {
	weights := make([]float64, n)
	for i := 0; i < n; i++ {
		weights[i] = math.Pow(decay, float64(n-1-i))
	}
	return weights
}

// weightedMean returns the weighted arithmetic mean of values.
// Returns 0 if the total weight is 0.
func weightedMean(values, weights []float64) float64 {
	var sumW, sumWX float64
	for i, v := range values {
		sumW += weights[i]
		sumWX += weights[i] * v
	}
	if sumW == 0 {
		return 0
	}
	return sumWX / sumW
}

// weightedStddev returns the weighted population standard deviation.
// Returns 0 if the total weight is 0.
func weightedStddev(values, weights []float64, avg float64) float64 {
	var sumW, sumWSq float64
	for i, v := range values {
		sumW += weights[i]
		diff := v - avg
		sumWSq += weights[i] * diff * diff
	}
	if sumW == 0 {
		return 0
	}
	return math.Sqrt(sumWSq / sumW)
}

// sortSessionsByTime sorts sessions oldest-first by StartTime (RFC3339 string
// comparison is correct for lexicographic ordering).
func sortSessionsByTime(sessions []claude.SessionMeta) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime < sessions[j].StartTime
	})
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
