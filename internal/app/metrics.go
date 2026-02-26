package app

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
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
	Days         int                       `json:"days"`
	Project      string                    `json:"project,omitempty"`
	Sessions     int                       `json:"total_sessions"`
	Velocity     analyzer.VelocityMetrics  `json:"velocity"`
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
	renderTokenEconomics(stats)
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

	// Friction trends.
	persistence := analyzer.AnalyzeFrictionPersistence(facets, sessions)
	renderFrictionTrends(persistence)

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

func renderTokenEconomics(stats *claude.StatsCache) {
	fmt.Println(output.Section("Token Economics"))

	if stats == nil {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render("No stats cache available"))
		return
	}

	var totalInput, totalOutput, totalCacheRead, totalCacheCreation int64
	var totalCost float64

	for _, mu := range stats.ModelUsage {
		totalInput += mu.InputTokens
		totalOutput += mu.OutputTokens
		totalCacheRead += mu.CacheReadInputTokens
		totalCacheCreation += mu.CacheCreationInputTokens
		totalCost += mu.CostUSD
	}

	totalTokens := totalInput + totalOutput
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Total tokens"),
		output.StyleValue.Render(formatTokenCount(totalTokens)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Input tokens"),
		output.StyleValue.Render(formatTokenCount(totalInput)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Output tokens"),
		output.StyleValue.Render(formatTokenCount(totalOutput)))

	// Cache hit ratio: cache reads as a percentage of cache-eligible input tokens
	// (cache reads + non-cached input). Cache creation tokens are excluded because
	// they represent new entries being written, not cache lookups.
	totalCacheEligible := totalCacheRead + totalInput
	if totalCacheEligible > 0 {
		cacheRatio := float64(totalCacheRead) / float64(totalCacheEligible) * 100.0
		if cacheRatio > 100.0 {
			cacheRatio = 100.0
		}
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Cache hit ratio"),
			output.StyleValue.Render(fmt.Sprintf("%.0f%%", cacheRatio)))
	}

	if totalCost > 0 {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Estimated cost"),
			output.StyleValue.Render(fmt.Sprintf("$%.2f", totalCost)))
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
