package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/spf13/cobra"
)

var (
	correlateFlagFactor  string
	correlateFlagProject string
)

var correlateCmd = &cobra.Command{
	Use:   "correlate <outcome>",
	Short: "Correlate session attributes against outcomes",
	Long: `Analyze which factors predict better or worse outcomes across your Claude Code sessions.
Supports Pearson correlation for numeric factors and grouped comparison for boolean factors.
Groups with fewer than 10 sessions are flagged as low-confidence.

Valid outcomes: friction, commits, zero_commit, cost, duration, tool_errors
Valid factors:  has_claude_md, uses_task_agent, uses_mcp, uses_web_search, is_saw,
                tool_call_count, duration, input_tokens

Examples:
  claudewatch correlate friction
  claudewatch correlate commits --factor has_claude_md
  claudewatch correlate cost --project claudewatch
  claudewatch correlate friction --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCorrelate,
}

func init() {
	correlateCmd.Flags().StringVar(&correlateFlagFactor, "factor", "", "Factor field to analyze (default: all factors)")
	correlateCmd.Flags().StringVar(&correlateFlagProject, "project", "", "Filter to a specific project by name")
	rootCmd.AddCommand(correlateCmd)
}

func runCorrelate(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		output.SetNoColor(true)
	}

	outcome := analyzer.OutcomeField(args[0])
	validOutcomes := []string{"friction", "commits", "zero_commit", "cost", "duration", "tool_errors"}
	switch outcome {
	case analyzer.OutcomeFriction, analyzer.OutcomeCommits, analyzer.OutcomeZeroCommit,
		analyzer.OutcomeCost, analyzer.OutcomeDuration, analyzer.OutcomeToolErrors:
		// valid
	default:
		return fmt.Errorf("unknown outcome %q; valid values: %s", outcome, strings.Join(validOutcomes, ", "))
	}

	var factor analyzer.FactorField
	if correlateFlagFactor != "" {
		factor = analyzer.FactorField(correlateFlagFactor)
		validFactors := []string{"has_claude_md", "uses_task_agent", "uses_mcp", "uses_web_search", "is_saw", "tool_call_count", "duration", "input_tokens"}
		switch factor {
		case analyzer.FactorHasClaudeMD, analyzer.FactorUsesTaskAgent, analyzer.FactorUsesMCP,
			analyzer.FactorUsesWebSearch, analyzer.FactorIsSAW, analyzer.FactorToolCallCount,
			analyzer.FactorDuration, analyzer.FactorInputTokens:
			// valid
		default:
			return fmt.Errorf("unknown factor %q; valid values: %s", factor, strings.Join(validFactors, ", "))
		}
	}

	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No sessions found."))
		return nil
	}

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		// Non-fatal: proceed with empty facets.
		facets = nil
	}

	// Parse SAW sessions from transcripts.
	spans, err := claude.ParseSessionTranscripts(cfg.ClaudeHome)
	if err != nil {
		spans = nil
	}
	sawSessions := claude.ComputeSAWWaves(spans)
	sawSessionIDs := make(map[string]bool)
	for _, ss := range sawSessions {
		sawSessionIDs[ss.SessionID] = true
	}

	// Build projectPathMap: sessionID -> ProjectPath.
	projectPathMap := make(map[string]string, len(sessions))
	for _, sess := range sessions {
		projectPathMap[sess.SessionID] = sess.ProjectPath
	}

	// Load cache ratio (non-fatal).
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
		cacheRatio = analyzer.ComputeCacheRatio(*sc)
	}

	input := analyzer.CorrelateInput{
		Sessions:    sessions,
		Facets:      facets,
		SAWSessions: sawSessionIDs,
		ProjectPath: projectPathMap,
		Pricing:     pricing,
		CacheRatio:  cacheRatio,
		Project:     correlateFlagProject,
		Outcome:     outcome,
		Factor:      factor,
	}

	report, err := analyzer.CorrelateFactors(input)
	if err != nil {
		fmt.Printf(" %s\n", output.StyleMuted.Render(fmt.Sprintf("Cannot run analysis: %s", err.Error())))
		return nil
	}

	if flagJSON {
		return json.NewEncoder(os.Stdout).Encode(report)
	}

	renderCorrelate(report)
	return nil
}

func renderCorrelate(report analyzer.FactorAnalysisReport) {
	title := fmt.Sprintf("Factor Analysis: %s", report.Outcome)
	if report.Project != "" {
		title = fmt.Sprintf("Factor Analysis: %s — %s", report.Outcome, report.Project)
	}
	fmt.Println(output.Section(title))
	fmt.Println()

	// Single-factor focused view.
	if report.SingleGroupComparison != nil {
		gc := report.SingleGroupComparison
		renderSingleGroupComparison(gc)
		fmt.Println()
		fmt.Printf(" %s %s\n", output.StyleLabel.Render("Summary"), output.StyleValue.Render(""))
		fmt.Printf("   %s\n", report.Summary)
		fmt.Println()
		return
	}

	if report.SinglePearson != nil {
		pr := report.SinglePearson
		renderSinglePearson(pr)
		fmt.Println()
		fmt.Printf("   %s\n", report.Summary)
		fmt.Println()
		return
	}

	// All-factors mode: group comparisons table.
	if len(report.GroupComparisons) > 0 {
		fmt.Printf(" %s\n\n", output.StyleBold.Render("Boolean Factors"))
		tbl := output.NewTable("Factor", "With (n)", "Avg", "Without (n)", "Avg", "Delta", "Confidence")
		for _, gc := range report.GroupComparisons {
			confStr := ""
			if gc.LowConfidence {
				confStr = output.StyleWarning.Render("low")
			}
			tbl.AddRow(
				string(gc.Factor),
				fmt.Sprintf("%d", gc.TrueGroup.N),
				fmt.Sprintf("%.2f", gc.TrueGroup.AvgOutcome),
				fmt.Sprintf("%d", gc.FalseGroup.N),
				fmt.Sprintf("%.2f", gc.FalseGroup.AvgOutcome),
				fmt.Sprintf("%+.2f", gc.Delta),
				confStr,
			)
		}
		tbl.Print()
		fmt.Println()
	}

	// All-factors mode: Pearson table.
	if len(report.PearsonResults) > 0 {
		fmt.Printf(" %s\n\n", output.StyleBold.Render("Numeric Factors (Pearson r)"))
		tbl := output.NewTable("Factor", "r", "n", "Confidence")
		for _, pr := range report.PearsonResults {
			confStr := ""
			if pr.LowConfidence {
				confStr = output.StyleWarning.Render("low")
			}
			tbl.AddRow(
				string(pr.Factor),
				fmt.Sprintf("%.3f", pr.R),
				fmt.Sprintf("%d", pr.N),
				confStr,
			)
		}
		tbl.Print()
		fmt.Println()
	}

	// Summary paragraph.
	fmt.Printf(" %s\n", output.StyleBold.Render("Summary"))
	fmt.Printf("   %s\n", report.Summary)
	fmt.Println()
}

func renderSingleGroupComparison(gc *analyzer.GroupComparison) {
	label := func(l, v string) {
		fmt.Printf(" %-24s %s\n", output.StyleLabel.Render(l), v)
	}

	label("Factor", output.StyleBold.Render(string(gc.Factor)))
	label("With factor (n)", fmt.Sprintf("%d", gc.TrueGroup.N))
	label("  avg outcome", fmt.Sprintf("%.2f", gc.TrueGroup.AvgOutcome))
	label("  std dev", fmt.Sprintf("%.2f", gc.TrueGroup.StdDev))
	label("Without factor (n)", fmt.Sprintf("%d", gc.FalseGroup.N))
	label("  avg outcome", fmt.Sprintf("%.2f", gc.FalseGroup.AvgOutcome))
	label("  std dev", fmt.Sprintf("%.2f", gc.FalseGroup.StdDev))
	delta := fmt.Sprintf("%+.2f", gc.Delta)
	label("Delta", delta)
	if gc.LowConfidence {
		label("Confidence", output.StyleWarning.Render("low (< 10 sessions in a group)"))
	} else {
		label("Confidence", "ok")
	}
	if gc.Note != "" {
		label("Note", gc.Note)
	}
}

func renderSinglePearson(pr *analyzer.PearsonResult) {
	label := func(l, v string) {
		fmt.Printf(" %-24s %s\n", output.StyleLabel.Render(l), v)
	}

	label("Factor", output.StyleBold.Render(string(pr.Factor)))
	label("Pearson r", fmt.Sprintf("%.3f", pr.R))
	label("n", fmt.Sprintf("%d", pr.N))
	if pr.LowConfidence {
		label("Confidence", output.StyleWarning.Render("low (< 10 sessions)"))
	} else {
		label("Confidence", "ok")
	}
	if pr.Note != "" {
		label("Note", pr.Note)
	}
}
