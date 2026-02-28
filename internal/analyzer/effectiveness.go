package analyzer

import (
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// EffectivenessResult measures whether a CLAUDE.md change actually improved
// session outcomes. It compares metrics from sessions before the change to
// sessions after the change.
type EffectivenessResult struct {
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name"`

	// ChangeDetectedAt is the timestamp when the CLAUDE.md was last modified.
	ChangeDetectedAt time.Time `json:"change_detected_at"`

	// Before/after session counts.
	BeforeSessions int `json:"before_sessions"`
	AfterSessions  int `json:"after_sessions"`

	// Friction comparison.
	BeforeFrictionRate float64 `json:"before_friction_rate"`
	AfterFrictionRate  float64 `json:"after_friction_rate"`
	FrictionDelta      float64 `json:"friction_delta"`

	// Tool error comparison.
	BeforeToolErrors float64 `json:"before_tool_errors"`
	AfterToolErrors  float64 `json:"after_tool_errors"`
	ToolErrorDelta   float64 `json:"tool_error_delta"`

	// Interruption comparison.
	BeforeInterruptions float64 `json:"before_interruptions"`
	AfterInterruptions  float64 `json:"after_interruptions"`
	InterruptionDelta   float64 `json:"interruption_delta"`

	// Goal achievement comparison.
	BeforeGoalRate float64 `json:"before_goal_rate"`
	AfterGoalRate  float64 `json:"after_goal_rate"`
	GoalDelta      float64 `json:"goal_delta"`

	// Cost comparison.
	BeforeCostPerCommit float64 `json:"before_cost_per_commit"`
	AfterCostPerCommit  float64 `json:"after_cost_per_commit"`
	CostDelta           float64 `json:"cost_delta"`

	// Overall verdict.
	Verdict string `json:"verdict"`
	Score   int    `json:"score"`
}

// AnalyzeEffectiveness measures the before/after impact of a CLAUDE.md change
// on session quality metrics. It splits sessions at the CLAUDE.md modification
// time and compares the two groups.
func AnalyzeEffectiveness(
	projectPath string,
	claudeMDModTime time.Time,
	sessions []claude.SessionMeta,
	facets []claude.SessionFacet,
	pricing ModelPricing,
	ratio CacheRatio,
) EffectivenessResult {
	result := EffectivenessResult{
		ProjectPath:      projectPath,
		ProjectName:      projectNameFromPath(projectPath),
		ChangeDetectedAt: claudeMDModTime,
	}

	if claudeMDModTime.IsZero() || len(sessions) == 0 {
		result.Verdict = "insufficient_data"
		return result
	}

	// Index facets by session ID.
	facetBySession := make(map[string]claude.SessionFacet, len(facets))
	for _, f := range facets {
		facetBySession[f.SessionID] = f
	}

	// Split sessions into before and after the CLAUDE.md change.
	var before, after []claude.SessionMeta
	for _, s := range sessions {
		t := claude.ParseTimestamp(s.StartTime)
		if t.IsZero() {
			continue
		}
		if t.Before(claudeMDModTime) {
			before = append(before, s)
		} else {
			after = append(after, s)
		}
	}

	result.BeforeSessions = len(before)
	result.AfterSessions = len(after)

	if len(before) < 2 || len(after) < 2 {
		result.Verdict = "insufficient_data"
		return result
	}

	// Friction rate: avg friction events per session.
	result.BeforeFrictionRate = avgFrictionRate(before, facetBySession)
	result.AfterFrictionRate = avgFrictionRate(after, facetBySession)
	result.FrictionDelta = result.AfterFrictionRate - result.BeforeFrictionRate

	// Tool errors per session.
	result.BeforeToolErrors = avgToolErrors(before)
	result.AfterToolErrors = avgToolErrors(after)
	result.ToolErrorDelta = result.AfterToolErrors - result.BeforeToolErrors

	// Interruptions per session.
	result.BeforeInterruptions = avgInterruptions(before)
	result.AfterInterruptions = avgInterruptions(after)
	result.InterruptionDelta = result.AfterInterruptions - result.BeforeInterruptions

	// Goal achievement rate.
	result.BeforeGoalRate = goalRate(before, facetBySession)
	result.AfterGoalRate = goalRate(after, facetBySession)
	result.GoalDelta = result.AfterGoalRate - result.BeforeGoalRate

	// Cost per commit.
	result.BeforeCostPerCommit = costPerCommit(before, pricing, ratio)
	result.AfterCostPerCommit = costPerCommit(after, pricing, ratio)
	result.CostDelta = result.AfterCostPerCommit - result.BeforeCostPerCommit

	// Score: each improving metric contributes points. Range -100 to +100.
	result.Score, result.Verdict = computeEffectivenessScore(result)

	return result
}

// EffectivenessTimeline computes effectiveness results for every detected
// CLAUDE.md change across multiple projects, producing a timeline of changes
// and their measured impact.
func EffectivenessTimeline(
	changes []ClaudeMDChange,
	sessions []claude.SessionMeta,
	facets []claude.SessionFacet,
	pricing ModelPricing,
	ratio CacheRatio,
) []EffectivenessResult {
	// Index sessions by project path.
	sessionsByProject := make(map[string][]claude.SessionMeta)
	for _, s := range sessions {
		p := claude.NormalizePath(s.ProjectPath)
		sessionsByProject[p] = append(sessionsByProject[p], s)
	}

	// Sort each project's sessions by time.
	for _, ss := range sessionsByProject {
		sort.Slice(ss, func(i, j int) bool {
			return ss[i].StartTime < ss[j].StartTime
		})
	}

	var results []EffectivenessResult
	for _, change := range changes {
		p := claude.NormalizePath(change.ProjectPath)
		projectSessions := sessionsByProject[p]
		if len(projectSessions) == 0 {
			continue
		}

		r := AnalyzeEffectiveness(change.ProjectPath, change.ModifiedAt, projectSessions, facets, pricing, ratio)
		results = append(results, r)
	}

	// Sort by change time.
	sort.Slice(results, func(i, j int) bool {
		return results[i].ChangeDetectedAt.Before(results[j].ChangeDetectedAt)
	})

	return results
}

// ClaudeMDChange records when a project's CLAUDE.md was modified.
type ClaudeMDChange struct {
	ProjectPath string
	ModifiedAt  time.Time
}

func avgFrictionRate(sessions []claude.SessionMeta, facets map[string]claude.SessionFacet) float64 {
	if len(sessions) == 0 {
		return 0
	}
	total := 0
	count := 0
	for _, s := range sessions {
		f, ok := facets[s.SessionID]
		if !ok {
			continue
		}
		count++
		for _, c := range f.FrictionCounts {
			total += c
		}
	}
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func avgToolErrors(sessions []claude.SessionMeta) float64 {
	if len(sessions) == 0 {
		return 0
	}
	total := 0
	for _, s := range sessions {
		total += s.ToolErrors
	}
	return float64(total) / float64(len(sessions))
}

func avgInterruptions(sessions []claude.SessionMeta) float64 {
	if len(sessions) == 0 {
		return 0
	}
	total := 0
	for _, s := range sessions {
		total += s.UserInterruptions
	}
	return float64(total) / float64(len(sessions))
}

func goalRate(sessions []claude.SessionMeta, facets map[string]claude.SessionFacet) float64 {
	achieved := 0
	total := 0
	for _, s := range sessions {
		f, ok := facets[s.SessionID]
		if !ok {
			continue
		}
		if f.Outcome == "" {
			continue
		}
		total++
		if f.Outcome == "achieved" || f.Outcome == "mostly_achieved" {
			achieved++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(achieved) / float64(total)
}

func costPerCommit(sessions []claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64 {
	var totalCost float64
	var totalCommits int
	for _, s := range sessions {
		totalCost += estimateSessionCost(s, pricing, ratio)
		totalCommits += s.GitCommits
	}
	if totalCommits == 0 {
		return 0
	}
	return totalCost / float64(totalCommits)
}

// computeEffectivenessScore produces a -100 to +100 score from the deltas.
// Negative deltas in friction, errors, interruptions, and cost are good.
// Positive delta in goal rate is good.
func computeEffectivenessScore(r EffectivenessResult) (int, string) {
	score := 0

	// Friction: negative delta = improvement. Weight: 30 points.
	if r.BeforeFrictionRate > 0 {
		change := r.FrictionDelta / r.BeforeFrictionRate
		score += clampScore(int(-change * 30))
	}

	// Tool errors: negative delta = improvement. Weight: 20 points.
	if r.BeforeToolErrors > 0 {
		change := r.ToolErrorDelta / r.BeforeToolErrors
		score += clampScore(int(-change * 20))
	}

	// Interruptions: negative delta = improvement. Weight: 20 points.
	if r.BeforeInterruptions > 0 {
		change := r.InterruptionDelta / r.BeforeInterruptions
		score += clampScore(int(-change * 20))
	}

	// Goal rate: positive delta = improvement. Weight: 20 points.
	if r.BeforeGoalRate > 0 {
		change := r.GoalDelta / r.BeforeGoalRate
		score += clampScore(int(change * 20))
	}

	// Cost per commit: negative delta = improvement. Weight: 10 points.
	if r.BeforeCostPerCommit > 0 {
		change := r.CostDelta / r.BeforeCostPerCommit
		score += clampScore(int(-change * 10))
	}

	// Clamp total.
	if score > 100 {
		score = 100
	}
	if score < -100 {
		score = -100
	}

	var verdict string
	switch {
	case score >= 20:
		verdict = "effective"
	case score >= 0:
		verdict = "neutral"
	default:
		verdict = "regression"
	}

	return score, verdict
}

func clampScore(v int) int {
	if v > 100 {
		return 100
	}
	if v < -100 {
		return -100
	}
	return v
}
