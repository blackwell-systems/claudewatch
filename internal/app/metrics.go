package app

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
	"github.com/spf13/cobra"
)

var (
	metricsDays    int
	metricsProject string
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Parse session data and display trends",
	Long: `Analyze Claude Code session data to compute and display productivity,
efficiency, satisfaction, and agent performance metrics.

Metrics are computed from session-meta, facets, and agent task data.`,
	RunE: runMetrics,
}

func init() {
	metricsCmd.Flags().IntVar(&metricsDays, "days", 30, "Number of days to analyze")
	metricsCmd.Flags().StringVar(&metricsProject, "project", "", "Filter to a specific project path")
	metricsCmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(metricsCmd)
}

// metricsOutput is the JSON-serializable output for the metrics command.
type metricsOutput struct {
	Days         int                        `json:"days"`
	Project      string                     `json:"project,omitempty"`
	Sessions     int                        `json:"total_sessions"`
	Velocity     analyzer.VelocityMetrics   `json:"velocity"`
	Efficiency   analyzer.EfficiencyMetrics `json:"efficiency"`
	Satisfaction analyzer.SatisfactionScore `json:"satisfaction"`
	Agents       analyzer.AgentPerformance  `json:"agents"`
}

func runMetrics(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Load session meta data.
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	// Filter by project if specified.
	if metricsProject != "" {
		sessions = filterSessionsByProject(sessions, metricsProject)
	}

	// Load facets.
	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	if metricsProject != "" {
		facets = scanner.FilterFacetsByProject(facets, sessions, metricsProject)
	}

	// Load agent tasks from session transcripts.
	agentTasks, err := claude.ParseAgentTasks(cfg.ClaudeHome)
	if err != nil {
		// Non-fatal if transcript parsing fails.
		agentTasks = nil
	}

	// Load stats cache for token economics.
	stats, err := claude.ParseStatsCache(cfg.ClaudeHome)
	if err != nil {
		stats = nil
	}

	// Run analyzers.
	velocity := analyzer.AnalyzeVelocity(sessions, metricsDays)
	efficiency := analyzer.AnalyzeEfficiency(sessions)
	satisfaction := analyzer.AnalyzeSatisfaction(facets)
	agents := analyzer.AnalyzeAgents(agentTasks)

	// JSON output mode.
	if flagJSON {
		out := metricsOutput{
			Days:         metricsDays,
			Project:      metricsProject,
			Sessions:     len(sessions),
			Velocity:     velocity,
			Efficiency:   efficiency,
			Satisfaction: satisfaction,
			Agents:       agents,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Render styled output.
	renderSessionVolume(velocity, stats)
	renderProductivity(velocity)
	renderEfficiency(efficiency)
	renderSatisfaction(satisfaction)
	renderTokenUsage(stats, sessions)
	renderModelUsage(stats)
	renderFeatureAdoption(efficiency.FeatureAdoption)
	renderAgentPerformance(agents)

	// Cost estimation (augments token economics with detailed cost breakdown).
	if stats != nil {
		totalCommits := 0
		for _, s := range sessions {
			totalCommits += s.GitCommits
		}
		costEst := analyzer.EstimateCosts(*stats, "", len(sessions), totalCommits)
		renderCostEstimation(costEst)
	}

	// Commit patterns.
	commitAnalysis := analyzer.AnalyzeCommits(sessions)
	renderCommitPatterns(commitAnalysis)

	// Conversation quality.
	convAnalysis, err := analyzer.AnalyzeConversations(cfg.ClaudeHome)
	if err != nil {
		log.Printf("Warning: conversation analysis failed: %v", err)
	} else {
		renderConversationQuality(convAnalysis)
	}

	// Project confidence (edit-to-read ratio analysis).
	confidence := analyzer.AnalyzeConfidence(sessions)
	renderProjectConfidence(confidence)

	// Friction trends.
	persistence := analyzer.AnalyzeFrictionPersistence(facets, sessions)
	renderFrictionTrends(persistence)

	// Cost-per-outcome analysis.
	pricing := analyzer.DefaultPricing["sonnet"]
	outcomes := analyzer.AnalyzeOutcomes(sessions, facets, pricing)
	renderCostPerOutcome(outcomes)

	// CLAUDE.md effectiveness scoring.
	projects, projErr := scanner.DiscoverProjects(cfg.ScanPaths)
	if projErr == nil {
		changes := detectClaudeMDChanges(projects)
		if len(changes) > 0 {
			effectiveness := analyzer.EffectivenessTimeline(changes, sessions, facets, pricing)
			renderEffectiveness(effectiveness)
		}
	}

	return nil
}

func renderSessionVolume(v analyzer.VelocityMetrics, stats *claude.StatsCache) {
	fmt.Println(output.Section("Session Volume"))

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total sessions"),
		output.StyleValue.Render(fmt.Sprintf("%d", v.TotalSessions)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg duration"),
		output.StyleValue.Render(fmt.Sprintf("%.0f min", v.AvgDurationMinutes)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg messages/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", v.AvgMessagesPerSession)))

	if stats != nil {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Total messages (all)"),
			output.StyleValue.Render(fmt.Sprintf("%d", stats.TotalMessages)))
	}

	fmt.Println()
}

func renderProductivity(v analyzer.VelocityMetrics) {
	fmt.Println(output.Section("Productivity"))

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Lines added/session"),
		output.StyleValue.Render(fmt.Sprintf("%.0f", v.AvgLinesAddedPerSession)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Commits/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", v.AvgCommitsPerSession)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Files modified/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", v.AvgFilesModifiedPerSession)))
	fmt.Println()
}

func renderEfficiency(e analyzer.EfficiencyMetrics) {
	fmt.Println(output.Section("Efficiency"))

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Tool errors/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", e.AvgToolErrorsPerSession)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Interruptions/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", e.AvgInterruptionsPerSession)))

	// Show top error categories if any exist.
	if len(e.ErrorCategoryTotals) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("Error categories:"))
		sorted := sortMapByValue(e.ErrorCategoryTotals)
		for _, kv := range sorted {
			fmt.Printf("   %s %s\n",
				output.StyleLabel.Render(kv.key),
				output.StyleValue.Render(fmt.Sprintf("%d", kv.value)))
		}
	}

	// Show top tools by usage.
	if len(e.ToolUsageTotals) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("Tool call distribution:"))
		sorted := sortMapByValue(e.ToolUsageTotals)
		limit := 8
		if len(sorted) < limit {
			limit = len(sorted)
		}
		for _, kv := range sorted[:limit] {
			fmt.Printf("   %s %s\n",
				output.StyleLabel.Render(kv.key),
				output.StyleValue.Render(fmt.Sprintf("%d", kv.value)))
		}
	}

	fmt.Println()
}

func renderSatisfaction(s analyzer.SatisfactionScore) {
	fmt.Println(output.Section("Satisfaction"))

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Weighted score"),
		output.StyleValue.Render(fmt.Sprintf("%.0f/100", s.WeightedScore)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Facets analyzed"),
		output.StyleValue.Render(fmt.Sprintf("%d", s.TotalFacets)))

	if len(s.SatisfactionCounts) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("Satisfaction distribution:"))
		for level, count := range s.SatisfactionCounts {
			fmt.Printf("   %s %s\n",
				output.StyleLabel.Render(level),
				output.StyleValue.Render(fmt.Sprintf("%d", count)))
		}
	}

	if len(s.OutcomeCounts) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("Outcome distribution:"))
		for outcome, count := range s.OutcomeCounts {
			fmt.Printf("   %s %s\n",
				output.StyleLabel.Render(outcome),
				output.StyleValue.Render(fmt.Sprintf("%d", count)))
		}
	}

	fmt.Println()
}

func renderTokenUsage(stats *claude.StatsCache, sessions []claude.SessionMeta) {
	fmt.Println(output.Section("Token Usage"))

	ts := analyzer.AnalyzeTokens(stats, sessions)

	if ts.TotalTokens == 0 && ts.TotalSessions == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No token data available"))
		return
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total tokens"),
		output.StyleValue.Render(formatTokenCount(ts.TotalTokens)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Input"),
		output.StyleValue.Render(formatTokenCount(ts.TotalInput)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Output"),
		output.StyleValue.Render(formatTokenCount(ts.TotalOutput)))

	if ts.TotalCacheReads > 0 || ts.TotalCacheWrites > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cache reads"),
			output.StyleValue.Render(formatTokenCount(ts.TotalCacheReads)))
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cache writes"),
			output.StyleValue.Render(formatTokenCount(ts.TotalCacheWrites)))
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cache hit rate"),
			output.StyleValue.Render(fmt.Sprintf("%.0f%%", ts.CacheHitRate)))
	}

	if ts.InputOutputRatio > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Input/output ratio"),
			output.StyleValue.Render(fmt.Sprintf("%.1f:1", ts.InputOutputRatio)))
	}

	if ts.TotalSessions > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("Per session:"))
		fmt.Printf("   %s %s\n",
			output.StyleLabel.Render("Avg input"),
			output.StyleValue.Render(formatTokenCount(ts.AvgInputPerSession)))
		fmt.Printf("   %s %s\n",
			output.StyleLabel.Render("Avg output"),
			output.StyleValue.Render(formatTokenCount(ts.AvgOutputPerSession)))
		fmt.Printf("   %s %s\n",
			output.StyleLabel.Render("Avg total"),
			output.StyleValue.Render(formatTokenCount(ts.AvgTotalPerSession)))
	}

	fmt.Println()
}

func renderModelUsage(stats *claude.StatsCache) {
	ma := analyzer.AnalyzeModels(stats)

	fmt.Println(output.Section("Model Usage"))

	if len(ma.Models) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No model data available"))
		return
	}

	// Per-model breakdown.
	for _, m := range ma.Models {
		costLabel := fmt.Sprintf("$%.2f (%.0f%% of spend)", m.CostUSD, m.CostPercent)
		tokenLabel := fmt.Sprintf("%s tokens (%.0f%%)", formatTokenCount(m.TotalTokens), m.TokenPercent)
		fmt.Printf(" %s\n",
			output.StyleLabel.Render(m.ModelName))
		fmt.Printf("   %s  %s\n",
			output.StyleValue.Render(costLabel),
			output.StyleMuted.Render(tokenLabel))
	}

	// Tier summary.
	if len(ma.TierCosts) > 1 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("By tier:"))
		tiers := []analyzer.ModelTier{analyzer.TierOpus, analyzer.TierSonnet, analyzer.TierHaiku, analyzer.TierOther}
		for _, tier := range tiers {
			cost, ok := ma.TierCosts[tier]
			if !ok || cost == 0 {
				continue
			}
			pct := cost / ma.TotalCost * 100
			fmt.Printf("   %-8s $%.2f (%.0f%%)\n",
				string(tier), cost, pct)
		}
	}

	// Overspend signal.
	if ma.OpusCostPercent > 50 && ma.PotentialSavings > 1.0 {
		fmt.Printf("\n %s %s\n",
			output.StyleError.Render("⚠"),
			output.StyleLabel.Render(fmt.Sprintf(
				"%.0f%% of spend is on Opus. Switching to Sonnet could save $%.2f.",
				ma.OpusCostPercent, ma.PotentialSavings)))
	} else if ma.PotentialSavings > 0.01 {
		fmt.Printf("\n %s %s\n",
			output.StyleMuted.Render("Potential savings:"),
			output.StyleValue.Render(fmt.Sprintf("$%.2f if Opus usage moved to Sonnet", ma.PotentialSavings)))
	}

	fmt.Println()
}

func renderFeatureAdoption(fa analyzer.FeatureAdoption) {
	fmt.Println(output.Section("Feature Adoption"))

	if fa.TotalSessions == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No sessions to analyze"))
		return
	}

	total := float64(fa.TotalSessions)
	renderAdoptionLine("Task agents", fa.TaskAgentSessions, total)
	renderAdoptionLine("MCP", fa.MCPSessions, total)
	renderAdoptionLine("Web search", fa.WebSearchSessions, total)
	renderAdoptionLine("Web fetch", fa.WebFetchSessions, total)
	fmt.Println()
}

func renderAdoptionLine(name string, count int, total float64) {
	pct := float64(count) / total * 100.0
	fmt.Printf(" %s %s %s\n",
		output.StyleLabel.Render(name),
		output.StyleValue.Render(fmt.Sprintf("%d sessions", count)),
		output.StyleMuted.Render(fmt.Sprintf("(%.0f%%)", pct)))
}

func renderAgentPerformance(a analyzer.AgentPerformance) {
	fmt.Println(output.Section("Agent Performance"))

	if a.TotalAgents == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No agent tasks found in session transcripts"))
		return
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total agents spawned"),
		output.StyleValue.Render(fmt.Sprintf("%d", a.TotalAgents)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Success rate"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", a.SuccessRate*100)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Kill rate"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", a.KillRate*100)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Background ratio"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", a.BackgroundRatio*100)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg duration"),
		output.StyleValue.Render(fmt.Sprintf("%.0fs", a.AvgDurationMs/1000)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Parallel sessions"),
		output.StyleValue.Render(fmt.Sprintf("%d", a.ParallelSessions)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg tokens/agent"),
		output.StyleValue.Render(formatTokenCount(int64(a.AvgTokensPerAgent))))

	if len(a.ByType) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("By type:"))

		// Sort types by count descending.
		type typeEntry struct {
			name  string
			stats analyzer.AgentTypeStats
		}
		var entries []typeEntry
		for name, stats := range a.ByType {
			entries = append(entries, typeEntry{name, stats})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].stats.Count > entries[j].stats.Count
		})

		for _, e := range entries {
			fmt.Printf("   %-20s %3d  (%3.0f%% success)  avg %.0fs\n",
				e.name, e.stats.Count, e.stats.SuccessRate*100, e.stats.AvgDurationMs/1000)
		}
	}

	fmt.Println()
}

// formatTokenCount formats large token counts with K/M suffixes.
func formatTokenCount(tokens int64) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// kvPair is a key-value pair for sorted map iteration.
type kvPair struct {
	key   string
	value int
}

// sortMapByValue returns a slice of key-value pairs sorted by value descending.
func sortMapByValue(m map[string]int) []kvPair {
	pairs := make([]kvPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kvPair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].value > pairs[j].value
	})
	return pairs
}

func renderCostEstimation(est analyzer.CostEstimate) {
	fmt.Println(output.Section("Cost Estimation"))

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total cost"),
		output.StyleValue.Render(fmt.Sprintf("$%.2f", est.TotalCost)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Cost/session"),
		output.StyleValue.Render(fmt.Sprintf("$%.2f", est.CostPerSession)))

	if !math.IsInf(est.CostPerCommit, 0) {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cost/commit"),
			output.StyleValue.Render(fmt.Sprintf("$%.2f", est.CostPerCommit)))
	} else {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cost/commit"),
			output.StyleMuted.Render("N/A (no commits)"))
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Cache savings"),
		output.StyleValue.Render(fmt.Sprintf("$%.2f", est.CacheSavings)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Savings %"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", est.CacheSavingsPercent)))

	fmt.Println()
}

func renderCommitPatterns(ca analyzer.CommitAnalysis) {
	fmt.Println(output.Section("Commit Patterns"))

	if ca.TotalSessions == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No sessions to analyze"))
		return
	}

	zeroCommitPct := ca.ZeroCommitRate * 100
	zeroCommitLabel := fmt.Sprintf("%.0f%%", zeroCommitPct)
	if zeroCommitPct > 30 {
		zeroCommitLabel = output.StyleError.Render(fmt.Sprintf("%.0f%% ⚠", zeroCommitPct))
	}
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Zero-commit rate"),
		zeroCommitLabel)
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg commits/session"),
		output.StyleValue.Render(fmt.Sprintf("%.1f", ca.AvgCommitsPerSession)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Max commits (session)"),
		output.StyleValue.Render(fmt.Sprintf("%d", ca.MaxCommitsInSession)))

	fmt.Println()
}

func renderConversationQuality(ca analyzer.ConversationAnalysis) {
	fmt.Println(output.Section("Conversation Quality"))

	if len(ca.Sessions) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No conversation data available"))
		return
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg correction rate"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", ca.AvgCorrectionRate*100)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("High-correction sessions"),
		output.StyleValue.Render(fmt.Sprintf("%d", ca.HighCorrectionSessions)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Avg long message rate"),
		output.StyleValue.Render(fmt.Sprintf("%.0f%%", ca.AvgLongMsgRate*100)))

	fmt.Println()
}

func renderFrictionTrends(pa analyzer.PersistenceAnalysis) {
	fmt.Println(output.Section("Friction Trends"))

	if len(pa.Patterns) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No friction persistence data"))
		return
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Stale friction"),
		output.StyleValue.Render(fmt.Sprintf("%d", pa.StaleCount)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Improving"),
		output.StyleValue.Render(fmt.Sprintf("%d", pa.ImprovingCount)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Worsening"),
		output.StyleValue.Render(fmt.Sprintf("%d", pa.WorseningCount)))

	// Show top 3 stale patterns.
	staleShown := 0
	for _, p := range pa.Patterns {
		if !p.Stale || staleShown >= 3 {
			continue
		}
		fmt.Printf("\n  %s %s %s\n",
			output.StyleError.Render("⚠"),
			output.StyleLabel.Render(p.FrictionType),
			output.StyleMuted.Render(fmt.Sprintf("(%d consecutive weeks)", p.ConsecutiveWeeks)))
		staleShown++
	}

	fmt.Println()
}

func renderCostPerOutcome(o analyzer.OutcomeAnalysis) {
	fmt.Println(output.Section("Cost per Outcome"))

	if len(o.Sessions) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No sessions to analyze"))
		return
	}

	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total cost"),
		output.StyleValue.Render(fmt.Sprintf("$%.2f across %d sessions", o.TotalCost, len(o.Sessions))))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Cost/session"),
		output.StyleValue.Render(fmt.Sprintf("$%.2f", o.AvgCostPerSession)))

	if o.TotalCommits > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cost/commit"),
			output.StyleValue.Render(fmt.Sprintf("$%.2f (median $%.2f)", o.AvgCostPerCommit, o.MedianCostPerCommit)))
	}
	if o.TotalFilesModified > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cost/file modified"),
			output.StyleValue.Render(fmt.Sprintf("$%.2f", o.AvgCostPerFile)))
	}

	if o.GoalAchievementRate > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Goal achievement"),
			output.StyleValue.Render(fmt.Sprintf("%.0f%%", o.GoalAchievementRate*100)))

		achievedAvg, notAchievedAvg := analyzer.CostPerGoal(o)
		if achievedAvg > 0 && notAchievedAvg > 0 {
			fmt.Printf(" %s %s\n",
				output.StyleLabel.Render("Cost: achieved vs not"),
				output.StyleValue.Render(fmt.Sprintf("$%.2f vs $%.2f", achievedAvg, notAchievedAvg)))
		}
	}

	// Trend.
	if o.CostPerCommitTrend != "insufficient_data" {
		trendLabel := o.CostPerCommitTrend
		trendDetail := ""
		if o.TrendChangePercent != 0 {
			trendDetail = fmt.Sprintf(" (%.0f%%)", o.TrendChangePercent)
		}
		styled := output.StyleValue.Render(trendLabel + trendDetail)
		if o.CostPerCommitTrend == "improving" {
			styled = output.StyleSuccess.Render(trendLabel + trendDetail)
		} else if o.CostPerCommitTrend == "worsening" {
			styled = output.StyleError.Render(trendLabel + trendDetail)
		}
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cost/commit trend"),
			styled)
	}

	// Per-project breakdown (top 5).
	if len(o.ByProject) > 0 {
		fmt.Printf("\n %s\n", output.StyleMuted.Render("By project:"))
		limit := 5
		if len(o.ByProject) < limit {
			limit = len(o.ByProject)
		}
		for _, p := range o.ByProject[:limit] {
			cpc := "N/A"
			if p.TotalCommits > 0 {
				cpc = fmt.Sprintf("$%.2f/commit", p.CostPerCommit)
			}
			fmt.Printf("   %-24s $%.2f  (%d sessions, %s)\n",
				p.ProjectName, p.TotalCost, p.Sessions, cpc)
		}
	}

	fmt.Println()
}

func renderEffectiveness(results []analyzer.EffectivenessResult) {
	fmt.Println(output.Section("CLAUDE.md Effectiveness"))

	if len(results) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No CLAUDE.md changes detected with sufficient before/after data"))
		return
	}

	for _, r := range results {
		if r.Verdict == "insufficient_data" {
			continue
		}

		verdictStyled := output.StyleValue.Render(r.Verdict)
		switch r.Verdict {
		case "effective":
			verdictStyled = output.StyleSuccess.Render(r.Verdict)
		case "regression":
			verdictStyled = output.StyleError.Render(r.Verdict)
		}

		fmt.Printf(" %s %s  score: %d  %s\n",
			output.StyleLabel.Render(r.ProjectName),
			output.StyleMuted.Render(r.ChangeDetectedAt.Format("2006-01-02")),
			r.Score,
			verdictStyled)
		fmt.Printf("   %s %s  →  %s %s\n",
			output.StyleMuted.Render("friction"),
			formatDelta(r.BeforeFrictionRate, r.AfterFrictionRate, true),
			output.StyleMuted.Render("errors"),
			formatDelta(r.BeforeToolErrors, r.AfterToolErrors, true))
		fmt.Printf("   %s %s  →  %s %s\n",
			output.StyleMuted.Render("goals"),
			formatDelta(r.BeforeGoalRate*100, r.AfterGoalRate*100, false),
			output.StyleMuted.Render("cost/commit"),
			formatDelta(r.BeforeCostPerCommit, r.AfterCostPerCommit, true))
		fmt.Printf("   %s %d before, %d after\n",
			output.StyleMuted.Render("sessions:"),
			r.BeforeSessions, r.AfterSessions)
		fmt.Println()
	}
}

// formatDelta renders a before→after value with color. lowerIsBetter controls
// whether a decrease is green (good) or red (bad).
func formatDelta(before, after float64, lowerIsBetter bool) string {
	delta := after - before
	arrow := "→"
	label := fmt.Sprintf("%.1f %s %.1f", before, arrow, after)

	improved := (lowerIsBetter && delta < 0) || (!lowerIsBetter && delta > 0)
	if improved {
		return output.StyleSuccess.Render(label)
	}
	if delta == 0 {
		return output.StyleMuted.Render(label)
	}
	return output.StyleError.Render(label)
}

func renderProjectConfidence(ca analyzer.ConfidenceAnalysis) {
	fmt.Println(output.Section("Project Confidence"))

	if len(ca.Projects) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("Not enough session data for confidence analysis"))
		return
	}

	for _, pc := range ca.Projects {
		scoreStyled := output.StyleValue.Render(fmt.Sprintf("%.0f", pc.ConfidenceScore))
		if pc.ConfidenceScore < 40 {
			scoreStyled = output.StyleError.Render(fmt.Sprintf("%.0f", pc.ConfidenceScore))
		} else if pc.ConfidenceScore >= 70 {
			scoreStyled = output.StyleSuccess.Render(fmt.Sprintf("%.0f", pc.ConfidenceScore))
		}

		fmt.Printf(" %-24s score: %s  read: %.0f%%  write: %.0f%%  explore: %.0f%%\n",
			output.StyleLabel.Render(pc.ProjectName),
			scoreStyled,
			pc.AvgReadRatio*100,
			pc.AvgWriteRatio*100,
			pc.ExplorationRate*100)

		if pc.ConfidenceScore < 40 {
			fmt.Printf("   %s %s\n",
				output.StyleError.Render("⚠"),
				output.StyleMuted.Render(pc.Signal))
		}
	}

	if ca.LowConfidenceCount > 0 {
		fmt.Printf("\n %s\n",
			output.StyleMuted.Render(fmt.Sprintf(
				"%d project(s) with low confidence — consider adding more context to their CLAUDE.md",
				ca.LowConfidenceCount)))
	}

	fmt.Println()
}

// detectClaudeMDChanges finds projects with CLAUDE.md files and returns their
// modification times as change events for effectiveness analysis.
func detectClaudeMDChanges(projects []scanner.Project) []analyzer.ClaudeMDChange {
	var changes []analyzer.ClaudeMDChange
	for _, p := range projects {
		if !p.HasClaudeMD {
			continue
		}
		claudeMDPath := filepath.Join(p.Path, "CLAUDE.md")
		info, err := os.Stat(claudeMDPath)
		if err != nil {
			continue
		}
		changes = append(changes, analyzer.ClaudeMDChange{
			ProjectPath: p.Path,
			ModifiedAt:  info.ModTime(),
		})
	}
	return changes
}
