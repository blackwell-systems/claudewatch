// Package app contains the Cobra command tree for claudewatch.
package app

import (
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/spf13/cobra"
)

var appVersion = "dev"

// SetVersion sets the application version (called from main with ldflags value).
func SetVersion(v string) {
	appVersion = v
	rootCmd.Version = v
}

var (
	flagNoColor bool
	flagJSON    bool
	flagVerbose bool
	flagConfig  string
)

var rootCmd = &cobra.Command{
	Use:   "claudewatch",
	Short: "Observability for AI-assisted development workflows",
	Long: `claudewatch provides observability and continuous improvement for
Claude Code workflows. It reads local Claude data, analyzes session patterns,
scores project readiness, surfaces friction, and tracks improvement over time.

Run 'claudewatch' with no arguments to see a quick dashboard summary.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagNoColor {
			output.SetNoColor(true)
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
			renderEmptyDashboard()
			return nil
		}

		facets, _ := claude.ParseAllFacets(cfg.ClaudeHome)

		velocity := analyzer.AnalyzeVelocity(sessions, 30)
		satisfaction := analyzer.AnalyzeSatisfaction(facets)
		efficiency := analyzer.AnalyzeEfficiency(sessions)
		commits := analyzer.AnalyzeCommits(sessions)

		pricing := analyzer.DefaultPricing["sonnet"]
		cacheRatio := analyzer.NoCacheRatio()
		if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
			cacheRatio = analyzer.ComputeCacheRatio(*sc)
		}
		outcomes := analyzer.AnalyzeOutcomes(sessions, facets, pricing, cacheRatio)

		renderDashboard(velocity, satisfaction, efficiency, commits, outcomes)
		return nil
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Config file path (default: ~/.config/claudewatch/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
}

func renderDashboard(
	v analyzer.VelocityMetrics,
	s analyzer.SatisfactionScore,
	e analyzer.EfficiencyMetrics,
	c analyzer.CommitAnalysis,
	o analyzer.OutcomeAnalysis,
) {
	fmt.Printf(" %s %s\n",
		output.StyleBold.Render("claudewatch"),
		output.StyleMuted.Render(appVersion))

	fmt.Println(output.Section(fmt.Sprintf("Dashboard (%d sessions, last 30 days)", v.TotalSessions)))

	col1 := func(label, value string) string {
		return fmt.Sprintf(" %-24s %s", output.StyleLabel.Render(label), output.StyleValue.Render(value))
	}

	fmt.Println(col1("Sessions", fmt.Sprintf("%d", v.TotalSessions)))
	fmt.Println(col1("Avg duration", fmt.Sprintf("%.0f min", v.AvgDurationMinutes)))
	fmt.Println(col1("Commits/session", fmt.Sprintf("%.1f", v.AvgCommitsPerSession)))

	if s.TotalFacets > 0 {
		fmt.Println(col1("Satisfaction", fmt.Sprintf("%.0f/100", s.WeightedScore)))
	}

	fmt.Println(col1("Tool errors/session", fmt.Sprintf("%.1f", e.AvgToolErrorsPerSession)))

	if len(o.Sessions) > 0 {
		fmt.Println(col1("Cost/session", fmt.Sprintf("$%.2f", o.AvgCostPerSession)))
	}

	zeroLabel := fmt.Sprintf("%.0f%%", c.ZeroCommitRate*100)
	if c.ZeroCommitRate > 0.30 {
		zeroLabel = output.StyleError.Render(fmt.Sprintf("%.0f%%", c.ZeroCommitRate*100))
	}
	fmt.Println(col1("Zero-commit rate", zeroLabel))

	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch metrics    full analysis"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch gaps       find what's hurting you"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch suggest    ranked improvements"))
	fmt.Println()
}

func renderEmptyDashboard() {
	fmt.Printf(" %s %s\n",
		output.StyleBold.Render("claudewatch"),
		output.StyleMuted.Render(appVersion))
	fmt.Println()
	fmt.Println(" No session data found. Start using Claude Code, then run:")
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch scan       inventory your projects"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch metrics    session trends"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("claudewatch gaps       find friction patterns"))
	fmt.Println()
}
