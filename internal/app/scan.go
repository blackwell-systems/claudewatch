package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

var (
	scanFlagPaths    []string
	scanFlagMinScore float64
	scanFlagJSON     bool
	scanFlagSort     string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Inventory projects and score AI readiness",
	Long: `Scan discovers all Git projects in the configured scan paths,
checks each for Claude Code integration (CLAUDE.md, .claude/ directory,
session history), and computes a readiness score from 0-100.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringSliceVar(&scanFlagPaths, "path", nil, "Additional paths to scan (can be repeated)")
	scanCmd.Flags().Float64Var(&scanFlagMinScore, "min-score", 0, "Only show projects with score >= this value")
	scanCmd.Flags().BoolVar(&scanFlagJSON, "json", false, "Output as JSON")
	scanCmd.Flags().StringVar(&scanFlagSort, "sort", "score", "Sort by: score, name, sessions, last-active")

	rootCmd.AddCommand(scanCmd)
}

// scanResult holds the enriched project data for output.
type scanResult struct {
	scanner.Project
	Score float64 `json:"score"`
}

func runScan(cmd *cobra.Command, args []string) error {
	// Load configuration.
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Determine scan paths: config defaults + any --path flags.
	scanPaths := cfg.ScanPaths
	if len(scanFlagPaths) > 0 {
		scanPaths = append(scanPaths, scanFlagPaths...)
	}

	// Discover projects.
	projects, err := scanner.DiscoverProjects(scanPaths)
	if err != nil {
		return fmt.Errorf("discovering projects: %w", err)
	}

	// Parse Claude data.
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	settings, err := claude.ParseSettings(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}
	// Ensure settings is never nil for scoring.
	if settings == nil {
		settings = &claude.GlobalSettings{}
	}

	// Compute scores and enrich project data with session info.
	results := make([]scanResult, 0, len(projects))
	for i := range projects {
		p := &projects[i]
		score := scanner.ComputeReadiness(p, sessions, facets, settings)
		p.Score = score

		// Enrich with session count and last session date.
		projectSessions := filterSessionsByProject(sessions, p.Path)
		p.SessionCount = len(projectSessions)
		if len(projectSessions) > 0 {
			p.LastSessionDate = projectSessions[len(projectSessions)-1].StartTime
		}

		// Enrich with facets flag.
		p.HasFacets = len(scanner.FilterFacetsByProject(facets, sessions, p.Path)) > 0

		if score >= scanFlagMinScore {
			results = append(results, scanResult{Project: *p, Score: score})
		}
	}

	// Sort results.
	sortResults(results, scanFlagSort)

	// Render output.
	if scanFlagJSON || flagJSON {
		return renderScanJSON(results)
	}
	renderScanTable(results)
	renderScanSummary(results)
	return nil
}

func sortResults(results []scanResult, sortBy string) {
	sort.SliceStable(results, func(i, j int) bool {
		switch sortBy {
		case "name":
			return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
		case "sessions":
			return results[i].SessionCount > results[j].SessionCount
		case "last-active":
			return results[i].LastSessionDate > results[j].LastSessionDate
		default: // "score"
			return results[i].Score > results[j].Score
		}
	})
}

func renderScanJSON(results []scanResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func renderScanTable(results []scanResult) {
	fmt.Println(output.Section("Project Readiness Scan"))
	fmt.Println()

	tbl := output.NewTable("Score", "Project", "CLAUDE.md", "Sessions", "Last Active")

	for _, r := range results {
		// Score column.
		scoreStr := fmt.Sprintf("%5.0f", r.Score)

		// CLAUDE.md column.
		claudeMD := output.StyleError.Render("---")
		if r.HasClaudeMD {
			claudeMD = output.StyleSuccess.Render("yes")
		}

		// Sessions column.
		sessStr := fmt.Sprintf("%d", r.SessionCount)

		// Last Active column.
		lastActive := output.StyleMuted.Render("never")
		if r.LastSessionDate != "" {
			lastActive = formatRelativeTime(r.LastSessionDate)
		}

		tbl.AddRow(scoreStr, r.Name, claudeMD, sessStr, lastActive)
	}

	tbl.Print()
}

func renderScanSummary(results []scanResult) {
	if len(results) == 0 {
		fmt.Println(output.StyleMuted.Render("\n No projects found."))
		return
	}

	// Compute summary stats.
	var totalScore float64
	missingClaudeMD := 0
	coldStart := 0

	for _, r := range results {
		totalScore += r.Score
		if !r.HasClaudeMD {
			missingClaudeMD++
		}
		if r.SessionCount == 0 && !r.HasClaudeMD {
			coldStart++
		}
	}

	meanScore := totalScore / float64(len(results))

	fmt.Println(output.Section("Summary"))
	fmt.Println()
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Mean readiness:"),
		output.StyleValue.Render(fmt.Sprintf("%.0f/100", meanScore)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Projects scanned:"),
		output.StyleValue.Render(fmt.Sprintf("%d", len(results))))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Missing CLAUDE.md:"),
		output.StyleValue.Render(fmt.Sprintf("%d", missingClaudeMD)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Cold-start risk:"),
		output.StyleValue.Render(fmt.Sprintf("%d", coldStart)))
	fmt.Println()
}

// formatRelativeTime converts an RFC3339 timestamp to a human-friendly
// relative time string like "2d ago", "12h ago", "just now".
func formatRelativeTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		// Try date-only format.
		t, err = time.Parse("2006-01-02", timestamp)
		if err != nil {
			return timestamp
		}
	}

	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// filterSessionsByProject returns sessions whose ProjectPath matches the given path,
// sorted by StartTime ascending so the last element is the most recent.
func filterSessionsByProject(sessions []claude.SessionMeta, projectPath string) []claude.SessionMeta {
	normalized := claude.NormalizePath(projectPath)
	var result []claude.SessionMeta
	for _, s := range sessions {
		if claude.NormalizePath(s.ProjectPath) == normalized {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime < result[j].StartTime
	})
	return result
}
