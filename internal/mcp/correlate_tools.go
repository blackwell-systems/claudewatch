package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// CausalInsightsResult is the top-level result returned by get_causal_insights.
type CausalInsightsResult struct {
	Outcome               string                  `json:"outcome"`
	Project               string                  `json:"project,omitempty"`
	TotalSessions         int                     `json:"total_sessions"`
	GroupComparisons      []GroupComparisonResult `json:"group_comparisons,omitempty"`
	PearsonResults        []PearsonResultEntry    `json:"pearson_results,omitempty"`
	SingleGroupComparison *GroupComparisonResult  `json:"single_group_comparison,omitempty"`
	SinglePearson         *PearsonResultEntry     `json:"single_pearson,omitempty"`
	Summary               string                  `json:"summary"`
}

// GroupStatsResult holds aggregate outcome statistics for one boolean group.
type GroupStatsResult struct {
	N          int     `json:"n"`
	AvgOutcome float64 `json:"avg_outcome"`
	StdDev     float64 `json:"std_dev"`
}

// GroupComparisonResult compares outcome metrics between sessions where a boolean
// factor is true vs false.
type GroupComparisonResult struct {
	Factor        string           `json:"factor"`
	TrueGroup     GroupStatsResult `json:"true_group"`
	FalseGroup    GroupStatsResult `json:"false_group"`
	Delta         float64          `json:"delta"`
	LowConfidence bool             `json:"low_confidence"`
	Note          string           `json:"note"`
}

// PearsonResultEntry holds the Pearson correlation between a numeric factor and
// the outcome.
type PearsonResultEntry struct {
	Factor        string  `json:"factor"`
	R             float64 `json:"r"`
	N             int     `json:"n"`
	LowConfidence bool    `json:"low_confidence"`
	Note          string  `json:"note"`
}

// addCorrelateTools registers the get_causal_insights MCP tool on s.
func addCorrelateTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_causal_insights",
		Description: "Correlate session attributes against outcomes to identify what factors predict good sessions. Supports outcomes: friction, commits, zero_commit, cost, duration, tool_errors. Supports factors: has_claude_md, uses_task_agent, uses_mcp, uses_web_search, is_saw, tool_call_count, duration, input_tokens. Groups with n < 10 are flagged as low-confidence.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "outcome": {
                    "type": "string",
                    "description": "Outcome metric to analyze: friction, commits, zero_commit, cost, duration, tool_errors",
                    "enum": ["friction","commits","zero_commit","cost","duration","tool_errors"]
                },
                "factor": {
                    "type": "string",
                    "description": "Optional: specific factor to analyze. If omitted, all factors are analyzed.",
                    "enum": ["has_claude_md","uses_task_agent","uses_mcp","uses_web_search","is_saw","tool_call_count","duration","input_tokens"]
                },
                "project": {
                    "type": "string",
                    "description": "Optional: filter to a specific project by name."
                }
            },
            "required": ["outcome"],
            "additionalProperties": false
        }`),
		Handler: s.handleGetCausalInsights,
	})
}

// handleGetCausalInsights handles the get_causal_insights MCP tool.
func (s *Server) handleGetCausalInsights(args json.RawMessage) (any, error) {
	// Parse arguments.
	var params struct {
		Outcome *string `json:"outcome"`
		Factor  *string `json:"factor"`
		Project *string `json:"project"`
	}
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	// outcome is required.
	if params.Outcome == nil || *params.Outcome == "" {
		return nil, fmt.Errorf("outcome is required")
	}
	outcome := analyzer.OutcomeField(*params.Outcome)

	// Validate outcome value.
	switch outcome {
	case analyzer.OutcomeFriction, analyzer.OutcomeCommits, analyzer.OutcomeZeroCommit,
		analyzer.OutcomeCost, analyzer.OutcomeDuration, analyzer.OutcomeToolErrors:
		// valid
	default:
		return nil, fmt.Errorf("unrecognized outcome %q: must be one of friction, commits, zero_commit, cost, duration, tool_errors", outcome)
	}

	// Parse optional factor.
	var factor analyzer.FactorField
	if params.Factor != nil && *params.Factor != "" {
		factor = analyzer.FactorField(*params.Factor)
		// Validate factor value.
		switch factor {
		case analyzer.FactorHasClaudeMD, analyzer.FactorUsesTaskAgent, analyzer.FactorUsesMCP,
			analyzer.FactorUsesWebSearch, analyzer.FactorIsSAW, analyzer.FactorToolCallCount,
			analyzer.FactorDuration, analyzer.FactorInputTokens:
			// valid
		default:
			return nil, fmt.Errorf("unrecognized factor %q", factor)
		}
	}

	// Parse optional project.
	project := ""
	if params.Project != nil {
		project = *params.Project
	}

	// Load sessions (fatal on error).
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	// Load facets (non-fatal).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

	// Load SAW sessions (non-fatal on error — treat as empty map).
	sawSessionMap := make(map[string]bool)
	spans, err := claude.ParseSessionTranscripts(s.claudeHome)
	if err == nil {
		sawSessions := claude.ComputeSAWWaves(spans)
		for _, saw := range sawSessions {
			sawSessionMap[saw.SessionID] = true
		}
	}

	// Build projectPathMap: sessionID → ProjectPath.
	projectPathMap := make(map[string]string, len(sessions))
	for _, sess := range sessions {
		projectPathMap[sess.SessionID] = sess.ProjectPath
	}

	// Build CorrelateInput and call CorrelateFactors.
	input := analyzer.CorrelateInput{
		Sessions:    sessions,
		Facets:      facets,
		SAWSessions: sawSessionMap,
		ProjectPath: projectPathMap,
		Pricing:     analyzer.DefaultPricing["sonnet"],
		CacheRatio:  s.loadCacheRatio(),
		Project:     project,
		Outcome:     outcome,
		Factor:      factor,
	}

	report, err := analyzer.CorrelateFactors(input)
	if err != nil {
		return nil, err
	}

	// Map FactorAnalysisReport → CausalInsightsResult.
	result := CausalInsightsResult{
		Outcome:       string(report.Outcome),
		Project:       report.Project,
		TotalSessions: report.TotalSessions,
		Summary:       report.Summary,
	}

	if len(report.GroupComparisons) > 0 {
		result.GroupComparisons = make([]GroupComparisonResult, len(report.GroupComparisons))
		for i, gc := range report.GroupComparisons {
			result.GroupComparisons[i] = mapGroupComparison(gc)
		}
	}

	if len(report.PearsonResults) > 0 {
		result.PearsonResults = make([]PearsonResultEntry, len(report.PearsonResults))
		for i, pr := range report.PearsonResults {
			result.PearsonResults[i] = mapPearsonResult(pr)
		}
	}

	if report.SingleGroupComparison != nil {
		mapped := mapGroupComparison(*report.SingleGroupComparison)
		result.SingleGroupComparison = &mapped
	}

	if report.SinglePearson != nil {
		mapped := mapPearsonResult(*report.SinglePearson)
		result.SinglePearson = &mapped
	}

	return result, nil
}

// mapGroupComparison converts an analyzer.GroupComparison to a GroupComparisonResult.
func mapGroupComparison(gc analyzer.GroupComparison) GroupComparisonResult {
	return GroupComparisonResult{
		Factor: string(gc.Factor),
		TrueGroup: GroupStatsResult{
			N:          gc.TrueGroup.N,
			AvgOutcome: gc.TrueGroup.AvgOutcome,
			StdDev:     gc.TrueGroup.StdDev,
		},
		FalseGroup: GroupStatsResult{
			N:          gc.FalseGroup.N,
			AvgOutcome: gc.FalseGroup.AvgOutcome,
			StdDev:     gc.FalseGroup.StdDev,
		},
		Delta:         gc.Delta,
		LowConfidence: gc.LowConfidence,
		Note:          gc.Note,
	}
}

// mapPearsonResult converts an analyzer.PearsonResult to a PearsonResultEntry.
func mapPearsonResult(pr analyzer.PearsonResult) PearsonResultEntry {
	return PearsonResultEntry{
		Factor:        string(pr.Factor),
		R:             pr.R,
		N:             pr.N,
		LowConfidence: pr.LowConfidence,
		Note:          pr.Note,
	}
}
