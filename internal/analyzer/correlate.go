package analyzer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// OutcomeField identifies the metric used as the dependent variable.
type OutcomeField string

const (
	OutcomeFriction   OutcomeField = "friction"
	OutcomeCommits    OutcomeField = "commits"
	OutcomeZeroCommit OutcomeField = "zero_commit"
	OutcomeCost       OutcomeField = "cost"
	OutcomeDuration   OutcomeField = "duration"
	OutcomeToolErrors OutcomeField = "tool_errors"
)

// FactorField identifies the independent variable being analyzed.
type FactorField string

const (
	FactorHasClaudeMD   FactorField = "has_claude_md"
	FactorUsesTaskAgent FactorField = "uses_task_agent"
	FactorUsesMCP       FactorField = "uses_mcp"
	FactorUsesWebSearch FactorField = "uses_web_search"
	FactorIsSAW         FactorField = "is_saw"
	FactorToolCallCount FactorField = "tool_call_count"
	FactorDuration      FactorField = "duration"
	FactorInputTokens   FactorField = "input_tokens"
)

// GroupStats holds aggregate outcome statistics for one boolean group.
type GroupStats struct {
	N          int     `json:"n"`
	AvgOutcome float64 `json:"avg_outcome"`
	StdDev     float64 `json:"std_dev"`
}

// GroupComparison compares outcome metrics between sessions where a boolean
// factor is true vs false.
type GroupComparison struct {
	Factor        FactorField `json:"factor"`
	TrueGroup     GroupStats  `json:"true_group"`
	FalseGroup    GroupStats  `json:"false_group"`
	Delta         float64     `json:"delta"`
	LowConfidence bool        `json:"low_confidence"`
	Note          string      `json:"note"`
}

// PearsonResult holds the Pearson correlation between a numeric factor and
// the outcome.
type PearsonResult struct {
	Factor        FactorField `json:"factor"`
	R             float64     `json:"r"`
	N             int         `json:"n"`
	LowConfidence bool        `json:"low_confidence"`
	Note          string      `json:"note"`
}

// FactorAnalysisReport is the top-level result of a factor analysis run.
type FactorAnalysisReport struct {
	Outcome               OutcomeField      `json:"outcome"`
	Project               string            `json:"project"`
	TotalSessions         int               `json:"total_sessions"`
	GroupComparisons      []GroupComparison `json:"group_comparisons,omitempty"`
	PearsonResults        []PearsonResult   `json:"pearson_results,omitempty"`
	SingleGroupComparison *GroupComparison  `json:"single_group_comparison,omitempty"`
	SinglePearson         *PearsonResult    `json:"single_pearson,omitempty"`
	Summary               string            `json:"summary"`
}

// CorrelateInput is the data required to run a factor analysis.
type CorrelateInput struct {
	Sessions    []claude.SessionMeta
	Facets      []claude.SessionFacet
	SAWSessions map[string]bool
	ProjectPath map[string]string
	Pricing     ModelPricing
	CacheRatio  CacheRatio
	Project     string
	Outcome     OutcomeField
	Factor      FactorField
}

// booleanFactors is the ordered set of boolean factor fields.
var booleanFactors = []FactorField{
	FactorHasClaudeMD,
	FactorUsesTaskAgent,
	FactorUsesMCP,
	FactorUsesWebSearch,
	FactorIsSAW,
}

// numericFactors is the ordered set of numeric factor fields.
var numericFactors = []FactorField{
	FactorToolCallCount,
	FactorDuration,
	FactorInputTokens,
}

// validOutcomes is the set of recognized outcome field names.
var validOutcomes = map[OutcomeField]bool{
	OutcomeFriction:   true,
	OutcomeCommits:    true,
	OutcomeZeroCommit: true,
	OutcomeCost:       true,
	OutcomeDuration:   true,
	OutcomeToolErrors: true,
}

// validFactors is the set of recognized factor field names.
var validFactors = map[FactorField]bool{
	FactorHasClaudeMD:   true,
	FactorUsesTaskAgent: true,
	FactorUsesMCP:       true,
	FactorUsesWebSearch: true,
	FactorIsSAW:         true,
	FactorToolCallCount: true,
	FactorDuration:      true,
	FactorInputTokens:   true,
}

// isBooleanFactor returns true if the factor is boolean.
func isBooleanFactor(f FactorField) bool {
	for _, bf := range booleanFactors {
		if f == bf {
			return true
		}
	}
	return false
}

// localMean returns the arithmetic mean of a slice.
func localMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// localStddev returns the population standard deviation of a slice.
func localStddev(vals []float64, avg float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sumSq float64
	for _, v := range vals {
		d := v - avg
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)))
}

// pearsonR computes the Pearson correlation coefficient between x and y.
// Returns 0 if either slice has no variance or lengths differ.
func pearsonR(x, y []float64) float64 {
	n := len(x)
	if n != len(y) || n == 0 {
		return 0
	}
	xm := localMean(x)
	ym := localMean(y)

	var num, dxSq, dySq float64
	for i := 0; i < n; i++ {
		dx := x[i] - xm
		dy := y[i] - ym
		num += dx * dy
		dxSq += dx * dx
		dySq += dy * dy
	}
	denom := math.Sqrt(dxSq * dySq)
	if denom == 0 {
		return 0
	}
	return num / denom
}

// computeGroupStats builds a GroupStats from a slice of outcome values.
func computeGroupStats(outcomes []float64) GroupStats {
	n := len(outcomes)
	if n == 0 {
		return GroupStats{}
	}
	avg := localMean(outcomes)
	return GroupStats{
		N:          n,
		AvgOutcome: avg,
		StdDev:     localStddev(outcomes, avg),
	}
}

// outcomeValue returns the numeric outcome for a session given the outcome field.
func outcomeValue(
	sess claude.SessionMeta,
	outcome OutcomeField,
	facetsByID map[string]claude.SessionFacet,
	pricing ModelPricing,
	ratio CacheRatio,
) float64 {
	switch outcome {
	case OutcomeFriction:
		return float64(sessionFriction(sess, facetsByID))
	case OutcomeCommits:
		return float64(sess.GitCommits)
	case OutcomeZeroCommit:
		if sess.GitCommits == 0 {
			return 1.0
		}
		return 0.0
	case OutcomeCost:
		return EstimateSessionCost(sess, pricing, ratio)
	case OutcomeDuration:
		return float64(sess.DurationMinutes)
	case OutcomeToolErrors:
		return float64(sess.ToolErrors)
	}
	return 0
}

// boolFactorValue returns whether the boolean factor is true for this session.
func boolFactorValue(
	sess claude.SessionMeta,
	factor FactorField,
	sawSessions map[string]bool,
	projectPath map[string]string,
) bool {
	switch factor {
	case FactorHasClaudeMD:
		path, ok := projectPath[sess.SessionID]
		if !ok || path == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(path, "CLAUDE.md"))
		return err == nil
	case FactorUsesTaskAgent:
		return sess.UsesTaskAgent
	case FactorUsesMCP:
		return sess.UsesMCP
	case FactorUsesWebSearch:
		return sess.UsesWebSearch
	case FactorIsSAW:
		return sawSessions[sess.SessionID]
	}
	return false
}

// numericFactorValue returns the numeric factor value for a session.
func numericFactorValue(sess claude.SessionMeta, factor FactorField) float64 {
	switch factor {
	case FactorToolCallCount:
		var total int
		for _, v := range sess.ToolCounts {
			total += v
		}
		return float64(total)
	case FactorDuration:
		return float64(sess.DurationMinutes)
	case FactorInputTokens:
		return float64(sess.InputTokens)
	}
	return 0
}

// computeGroupComparison computes a GroupComparison for a single boolean factor.
func computeGroupComparison(
	sessions []claude.SessionMeta,
	factor FactorField,
	outcomes []float64,
	sawSessions map[string]bool,
	projectPath map[string]string,
) GroupComparison {
	var trueVals, falseVals []float64
	for i, sess := range sessions {
		if boolFactorValue(sess, factor, sawSessions, projectPath) {
			trueVals = append(trueVals, outcomes[i])
		} else {
			falseVals = append(falseVals, outcomes[i])
		}
	}

	trueStats := computeGroupStats(trueVals)
	falseStats := computeGroupStats(falseVals)
	delta := trueStats.AvgOutcome - falseStats.AvgOutcome
	lowConf := trueStats.N < 10 || falseStats.N < 10

	confSuffix := ""
	if lowConf {
		confSuffix = ", low-confidence"
	}
	note := fmt.Sprintf("%s sessions avg %.2f outcome vs %.2f (n=%d vs n=%d%s)",
		factor, trueStats.AvgOutcome, falseStats.AvgOutcome,
		trueStats.N, falseStats.N, confSuffix)

	return GroupComparison{
		Factor:        factor,
		TrueGroup:     trueStats,
		FalseGroup:    falseStats,
		Delta:         delta,
		LowConfidence: lowConf,
		Note:          note,
	}
}

// computePearsonResult computes a PearsonResult for a single numeric factor.
func computePearsonResult(
	sessions []claude.SessionMeta,
	factor FactorField,
	outcomes []float64,
) PearsonResult {
	n := len(sessions)
	xVals := make([]float64, n)
	for i, sess := range sessions {
		xVals[i] = numericFactorValue(sess, factor)
	}

	r := pearsonR(xVals, outcomes)
	lowConf := n < 10

	confSuffix := ""
	if lowConf {
		confSuffix = ", low-confidence"
	}
	note := fmt.Sprintf("%s has r=%.2f correlation with outcome (n=%d%s)",
		factor, r, n, confSuffix)

	return PearsonResult{
		Factor:        factor,
		R:             r,
		N:             n,
		LowConfidence: lowConf,
		Note:          note,
	}
}

// buildSummary generates a plain-English summary of the analysis results.
func buildSummary(
	groups []GroupComparison,
	pearsons []PearsonResult,
	single *GroupComparison,
	singleP *PearsonResult,
) string {
	// Single-factor mode
	if single != nil {
		confNote := ""
		if single.LowConfidence {
			confNote = " (low-confidence)"
		}
		return fmt.Sprintf("%s is the analyzed factor: sessions where it is true average %.2f outcome vs %.2f without (n=%d vs n=%d%s).",
			single.Factor, single.TrueGroup.AvgOutcome, single.FalseGroup.AvgOutcome,
			single.TrueGroup.N, single.FalseGroup.N, confNote)
	}
	if singleP != nil {
		confNote := ""
		if singleP.LowConfidence {
			confNote = " (low-confidence)"
		}
		return fmt.Sprintf("%s has r=%.2f correlation with the outcome (n=%d%s).",
			singleP.Factor, singleP.R, singleP.N, confNote)
	}

	// All-factors mode: find strongest group factor and strongest Pearson factor
	var summary string

	if len(groups) > 0 {
		best := groups[0]
		for _, g := range groups[1:] {
			if math.Abs(g.Delta) > math.Abs(best.Delta) {
				best = g
			}
		}
		confNote := ""
		if best.LowConfidence {
			confNote = ", low-confidence"
		}
		summary += fmt.Sprintf("%s is the strongest boolean factor: sessions with it true average %.2f outcome vs %.2f without (n=%d vs n=%d%s).",
			best.Factor, best.TrueGroup.AvgOutcome, best.FalseGroup.AvgOutcome,
			best.TrueGroup.N, best.FalseGroup.N, confNote)
	}

	if len(pearsons) > 0 {
		best := pearsons[0]
		for _, p := range pearsons[1:] {
			if math.Abs(p.R) > math.Abs(best.R) {
				best = p
			}
		}
		confNote := ""
		if best.LowConfidence {
			confNote = ", low-confidence"
		}
		if summary != "" {
			summary += " "
		}
		summary += fmt.Sprintf("%s shows the strongest numeric correlation with the outcome (r=%.2f, n=%d%s).",
			best.Factor, best.R, best.N, confNote)
	}

	if summary == "" {
		summary = "No factor data available."
	}
	return summary
}

// CorrelateFactors is the main entry point for factor analysis. It computes
// correlations between session factors and a specified outcome metric.
func CorrelateFactors(input CorrelateInput) (FactorAnalysisReport, error) {
	// Validate outcome.
	if !validOutcomes[input.Outcome] {
		return FactorAnalysisReport{}, fmt.Errorf("unknown outcome field %q", input.Outcome)
	}

	// Validate factor if specified.
	if input.Factor != "" && !validFactors[input.Factor] {
		return FactorAnalysisReport{}, fmt.Errorf("unknown factor field %q", input.Factor)
	}

	// Filter sessions by project.
	sessions := input.Sessions
	if input.Project != "" {
		filtered := sessions[:0:0]
		for _, sess := range sessions {
			if filepath.Base(sess.ProjectPath) == input.Project {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}

	// Require minimum data.
	if len(sessions) < 3 {
		return FactorAnalysisReport{}, fmt.Errorf(
			"insufficient sessions for factor analysis: need at least 3, got %d", len(sessions))
	}

	// Build facet index.
	facetsByID := buildFacetIndex(input.Facets)

	// Compute outcome vector.
	outcomes := make([]float64, len(sessions))
	for i, sess := range sessions {
		outcomes[i] = outcomeValue(sess, input.Outcome, facetsByID, input.Pricing, input.CacheRatio)
	}

	report := FactorAnalysisReport{
		Outcome:       input.Outcome,
		Project:       input.Project,
		TotalSessions: len(sessions),
	}

	if input.Factor != "" {
		// Single-factor mode.
		if isBooleanFactor(input.Factor) {
			gc := computeGroupComparison(sessions, input.Factor, outcomes, input.SAWSessions, input.ProjectPath)
			report.SingleGroupComparison = &gc
		} else {
			pr := computePearsonResult(sessions, input.Factor, outcomes)
			report.SinglePearson = &pr
		}
	} else {
		// All-factors mode.
		for _, bf := range booleanFactors {
			gc := computeGroupComparison(sessions, bf, outcomes, input.SAWSessions, input.ProjectPath)
			report.GroupComparisons = append(report.GroupComparisons, gc)
		}
		for _, nf := range numericFactors {
			pr := computePearsonResult(sessions, nf, outcomes)
			report.PearsonResults = append(report.PearsonResults, pr)
		}
	}

	report.Summary = buildSummary(
		report.GroupComparisons,
		report.PearsonResults,
		report.SingleGroupComparison,
		report.SinglePearson,
	)

	return report, nil
}
