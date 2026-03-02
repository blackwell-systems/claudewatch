package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/spf13/cobra"
)

var (
	compareFlagProject string
)

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare SAW vs sequential sessions side by side",
	Long: `Compare Scout-and-Wave (SAW) sessions against sequential sessions for a project.
Shows aggregate cost, commits, cost-per-commit, and friction for each workflow type.

If --project is not specified, the project from the most recent session is used.

Examples:
  claudewatch compare
  claudewatch compare --project claudewatch
  claudewatch compare --json`,
	RunE: runCompare,
}

func init() {
	compareCmd.Flags().StringVar(&compareFlagProject, "project", "", "Project name to compare (default: most recent session's project)")
	rootCmd.AddCommand(compareCmd)
}

func runCompare(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No sessions found."))
		return nil
	}

	// Determine project: use flag or derive from most recent session.
	project := compareFlagProject
	if project == "" {
		// Sort by start time descending, pick the first.
		sorted := make([]claude.SessionMeta, len(sessions))
		copy(sorted, sessions)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartTime > sorted[j].StartTime
		})
		project = filepath.Base(sorted[0].ProjectPath)
	}

	// Filter sessions to the requested project.
	var projectSessions []claude.SessionMeta
	for _, s := range sessions {
		name := filepath.Base(s.ProjectPath)
		if strings.EqualFold(name, project) ||
			strings.Contains(strings.ToLower(s.ProjectPath), strings.ToLower(project)) {
			projectSessions = append(projectSessions, s)
		}
	}

	if len(projectSessions) == 0 {
		return fmt.Errorf("no sessions found for project %q", project)
	}

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	// Parse SAW sessions from transcripts.
	spans, err := claude.ParseSessionTranscripts(cfg.ClaudeHome)
	if err != nil {
		// Non-fatal: proceed with no SAW sessions.
		spans = nil
	}

	sawSessions := claude.ComputeSAWWaves(spans)

	// Build sawSessionIDs (sessionID -> wave count) and sawAgentCounts (sessionID -> total agents).
	sawSessionIDs := make(map[string]int)
	sawAgentCounts := make(map[string]int)
	for _, ss := range sawSessions {
		sawSessionIDs[ss.SessionID] = len(ss.Waves)
		sawAgentCounts[ss.SessionID] = ss.TotalAgents
	}

	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
		cacheRatio = analyzer.ComputeCacheRatio(*sc)
	}

	report := analyzer.CompareSAWVsSequential(
		project,
		projectSessions,
		facets,
		sawSessionIDs,
		sawAgentCounts,
		pricing,
		cacheRatio,
		false,
	)

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	renderCompare(report)
	return nil
}

func renderCompare(report analyzer.ComparisonReport) {
	fmt.Println(output.Section(fmt.Sprintf("SAW vs Sequential — %s", report.Project)))
	fmt.Println()

	tbl := output.NewTable("Type", "Sessions", "Avg Cost", "Avg Commits", "Cost/Commit", "Avg Friction")

	// SAW row.
	sawCostPerCommit := "N/A"
	if report.SAW.CostPerCommit > 0 {
		sawCostPerCommit = fmt.Sprintf("$%.3f", report.SAW.CostPerCommit)
	}
	tbl.AddRow(
		output.StyleBold.Render("SAW"),
		fmt.Sprintf("%d", report.SAW.Count),
		fmt.Sprintf("$%.3f", report.SAW.AvgCostUSD),
		fmt.Sprintf("%.1f", report.SAW.AvgCommits),
		sawCostPerCommit,
		fmt.Sprintf("%.1f", report.SAW.AvgFriction),
	)

	// Sequential row.
	seqCostPerCommit := "N/A"
	if report.Sequential.CostPerCommit > 0 {
		seqCostPerCommit = fmt.Sprintf("$%.3f", report.Sequential.CostPerCommit)
	}
	tbl.AddRow(
		"Sequential",
		fmt.Sprintf("%d", report.Sequential.Count),
		fmt.Sprintf("$%.3f", report.Sequential.AvgCostUSD),
		fmt.Sprintf("%.1f", report.Sequential.AvgCommits),
		seqCostPerCommit,
		fmt.Sprintf("%.1f", report.Sequential.AvgFriction),
	)

	tbl.Print()

	// Totals footer.
	totalSessions := report.SAW.Count + report.Sequential.Count
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleBold.Render(fmt.Sprintf(
		"Total: %d sessions (%d SAW, %d sequential)",
		totalSessions, report.SAW.Count, report.Sequential.Count,
	)))
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --project <name> to switch project, --json for machine output"))
	fmt.Println()
}
