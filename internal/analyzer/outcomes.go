package analyzer

import (
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// SessionOutcome pairs a session's cost with what it accomplished.
type SessionOutcome struct {
	SessionID     string  `json:"session_id"`
	ProjectPath   string  `json:"project_path"`
	Cost          float64 `json:"cost"`
	Commits       int     `json:"commits"`
	LinesAdded    int     `json:"lines_added"`
	FilesModified int     `json:"files_modified"`
	GoalAchieved  bool    `json:"goal_achieved"`
	Outcome       string  `json:"outcome"`

	// Derived
	CostPerCommit float64 `json:"cost_per_commit"`
	CostPerFile   float64 `json:"cost_per_file"`
}

// OutcomeAnalysis is the top-level result of cost-per-outcome analysis.
type OutcomeAnalysis struct {
	Sessions []SessionOutcome `json:"sessions"`

	// Aggregates
	TotalCost           float64 `json:"total_cost"`
	TotalCommits        int     `json:"total_commits"`
	TotalFilesModified  int     `json:"total_files_modified"`
	TotalLinesAdded     int     `json:"total_lines_added"`
	GoalAchievementRate float64 `json:"goal_achievement_rate"`

	AvgCostPerCommit    float64 `json:"avg_cost_per_commit"`
	AvgCostPerFile      float64 `json:"avg_cost_per_file"`
	AvgCostPerSession   float64 `json:"avg_cost_per_session"`
	MedianCostPerCommit float64 `json:"median_cost_per_commit"`

	// Trend: compare first half vs second half of sessions by time.
	CostPerCommitTrend string  `json:"cost_per_commit_trend"`
	TrendChangePercent float64 `json:"trend_change_percent"`

	// Per-project breakdown.
	ByProject []ProjectOutcome `json:"by_project"`
}

// ProjectOutcome aggregates cost-per-outcome for a single project.
type ProjectOutcome struct {
	ProjectPath      string  `json:"project_path"`
	ProjectName      string  `json:"project_name"`
	Sessions         int     `json:"sessions"`
	TotalCost        float64 `json:"total_cost"`
	TotalCommits     int     `json:"total_commits"`
	CostPerCommit    float64 `json:"cost_per_commit"`
	CostPerSession   float64 `json:"cost_per_session"`
	GoalAchievedRate float64 `json:"goal_achieved_rate"`
}

// AnalyzeOutcomes computes cost-per-outcome metrics by joining session metadata
// with facet data and token-based cost estimates.
func AnalyzeOutcomes(sessions []claude.SessionMeta, facets []claude.SessionFacet, pricing ModelPricing, ratio CacheRatio) OutcomeAnalysis {
	result := OutcomeAnalysis{}

	if len(sessions) == 0 {
		return result
	}

	// Index facets by session ID.
	facetBySession := make(map[string]claude.SessionFacet, len(facets))
	for _, f := range facets {
		facetBySession[f.SessionID] = f
	}

	// Sort sessions by start time for trend analysis.
	sorted := make([]claude.SessionMeta, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime < sorted[j].StartTime
	})

	// Build per-session outcomes.
	for _, s := range sorted {
		cost := EstimateSessionCost(s, pricing, ratio)

		outcome := SessionOutcome{
			SessionID:     s.SessionID,
			ProjectPath:   s.ProjectPath,
			Cost:          cost,
			Commits:       s.GitCommits,
			LinesAdded:    s.LinesAdded,
			FilesModified: s.FilesModified,
		}

		// Enrich with facet data if available.
		if f, ok := facetBySession[s.SessionID]; ok {
			outcome.Outcome = f.Outcome
			outcome.GoalAchieved = f.Outcome == "achieved" || f.Outcome == "mostly_achieved"
		}

		// Derived costs.
		if s.GitCommits > 0 {
			outcome.CostPerCommit = cost / float64(s.GitCommits)
		}
		if s.FilesModified > 0 {
			outcome.CostPerFile = cost / float64(s.FilesModified)
		}

		result.Sessions = append(result.Sessions, outcome)
		result.TotalCost += cost
		result.TotalCommits += s.GitCommits
		result.TotalFilesModified += s.FilesModified
		result.TotalLinesAdded += s.LinesAdded
	}

	n := len(result.Sessions)

	// Aggregate averages.
	result.AvgCostPerSession = result.TotalCost / float64(n)
	if result.TotalCommits > 0 {
		result.AvgCostPerCommit = result.TotalCost / float64(result.TotalCommits)
	}
	if result.TotalFilesModified > 0 {
		result.AvgCostPerFile = result.TotalCost / float64(result.TotalFilesModified)
	}

	// Goal achievement rate.
	goalsAchieved := 0
	goalsTotal := 0
	for _, so := range result.Sessions {
		if so.Outcome != "" {
			goalsTotal++
			if so.GoalAchieved {
				goalsAchieved++
			}
		}
	}
	if goalsTotal > 0 {
		result.GoalAchievementRate = float64(goalsAchieved) / float64(goalsTotal)
	}

	// Median cost per commit (only sessions with commits).
	var costsPerCommit []float64
	for _, so := range result.Sessions {
		if so.Commits > 0 {
			costsPerCommit = append(costsPerCommit, so.CostPerCommit)
		}
	}
	result.MedianCostPerCommit = medianFloat64(costsPerCommit)

	// Trend: split sessions in half by time, compare avg cost-per-commit.
	result.CostPerCommitTrend, result.TrendChangePercent = computeOutcomeTrend(result.Sessions)

	// Per-project breakdown.
	result.ByProject = computeProjectOutcomes(result.Sessions)

	return result
}

// EstimateSessionCost computes the dollar cost of a single session from its
// token counts. Input tokens are treated as uncached; estimated cache-read and
// cache-write volumes are derived from the aggregate CacheRatio.
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64 {
	inputTokens := float64(s.InputTokens)
	uncachedCost := inputTokens / 1_000_000.0 * pricing.InputPerMillion
	cacheReadCost := (inputTokens * ratio.CacheReadMultiplier) / 1_000_000.0 * pricing.CacheReadPerMillion
	cacheWriteCost := (inputTokens * ratio.CacheWriteMultiplier) / 1_000_000.0 * pricing.CacheWritePerMillion
	outputCost := float64(s.OutputTokens) / 1_000_000.0 * pricing.OutputPerMillion
	return uncachedCost + cacheReadCost + cacheWriteCost + outputCost
}

// computeOutcomeTrend splits sessions in half by time and compares the average
// cost-per-commit of the two halves. Returns trend direction and percent change.
func computeOutcomeTrend(sessions []SessionOutcome) (string, float64) {
	if len(sessions) < 4 {
		return "insufficient_data", 0
	}

	mid := len(sessions) / 2
	earlierCPC := avgCostPerCommit(sessions[:mid])
	laterCPC := avgCostPerCommit(sessions[mid:])

	if earlierCPC == 0 {
		if laterCPC == 0 {
			return "stable", 0
		}
		return "worsening", 0
	}

	change := ((laterCPC - earlierCPC) / earlierCPC) * 100

	switch {
	case change < -10:
		return "improving", change
	case change > 10:
		return "worsening", change
	default:
		return "stable", change
	}
}

// avgCostPerCommit computes average cost per commit across sessions that have
// at least one commit.
func avgCostPerCommit(sessions []SessionOutcome) float64 {
	var totalCost float64
	var totalCommits int
	for _, s := range sessions {
		if s.Commits > 0 {
			totalCost += s.Cost
			totalCommits += s.Commits
		}
	}
	if totalCommits == 0 {
		return 0
	}
	return totalCost / float64(totalCommits)
}

// computeProjectOutcomes groups sessions by project and computes per-project
// cost-per-outcome aggregates.
func computeProjectOutcomes(sessions []SessionOutcome) []ProjectOutcome {
	type accum struct {
		cost          float64
		commits       int
		sessions      int
		goalsAchieved int
		goalsTotal    int
	}

	byProject := make(map[string]*accum)
	for _, s := range sessions {
		a, ok := byProject[s.ProjectPath]
		if !ok {
			a = &accum{}
			byProject[s.ProjectPath] = a
		}
		a.cost += s.Cost
		a.commits += s.Commits
		a.sessions++
		if s.Outcome != "" {
			a.goalsTotal++
			if s.GoalAchieved {
				a.goalsAchieved++
			}
		}
	}

	var results []ProjectOutcome
	for path, a := range byProject {
		po := ProjectOutcome{
			ProjectPath:  path,
			ProjectName:  projectNameFromPath(path),
			Sessions:     a.sessions,
			TotalCost:    a.cost,
			TotalCommits: a.commits,
		}
		if a.commits > 0 {
			po.CostPerCommit = a.cost / float64(a.commits)
		}
		if a.sessions > 0 {
			po.CostPerSession = a.cost / float64(a.sessions)
		}
		if a.goalsTotal > 0 {
			po.GoalAchievedRate = float64(a.goalsAchieved) / float64(a.goalsTotal)
		}
		results = append(results, po)
	}

	// Sort by total cost descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalCost > results[j].TotalCost
	})

	return results
}

// projectNameFromPath extracts the last path component as the project name.
func projectNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// medianFloat64 returns the median of a sorted float64 slice.
func medianFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	n := len(vals)
	if n%2 == 0 {
		return (vals[n/2-1] + vals[n/2]) / 2
	}
	return vals[n/2]
}

// CostPerGoal returns the average cost of sessions that achieved their goal vs
// those that didn't. Useful for showing "successful sessions cost $X, failed
// sessions cost $Y."
func CostPerGoal(outcomes OutcomeAnalysis) (achievedAvg, notAchievedAvg float64) {
	var achievedTotal, notAchievedTotal float64
	var achievedCount, notAchievedCount int

	for _, s := range outcomes.Sessions {
		if s.Outcome == "" {
			continue
		}
		if s.GoalAchieved {
			achievedTotal += s.Cost
			achievedCount++
		} else {
			notAchievedTotal += s.Cost
			notAchievedCount++
		}
	}

	if achievedCount > 0 {
		achievedAvg = achievedTotal / float64(achievedCount)
	}
	if notAchievedCount > 0 {
		notAchievedAvg = notAchievedTotal / float64(notAchievedCount)
	}

	return
}
