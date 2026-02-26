package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
	"github.com/spf13/cobra"
)

var gapsCmd = &cobra.Command{
	Use:   "gaps",
	Short: "Surface friction patterns and missing configuration",
	Long: `Analyze Claude Code usage data to identify gaps in configuration,
recurring friction patterns, missing hooks, unused skills, and
project-specific friction.`,
	RunE: runGaps,
}

func init() {
	gapsCmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(gapsCmd)
}

// gap represents a single identified gap or issue.
type gap struct {
	Severity string `json:"severity"` // critical, warning, info
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Project  string `json:"project,omitempty"`
}

// gapsOutput is the JSON-serializable output for the gaps command.
type gapsOutput struct {
	Gaps       []gap                    `json:"gaps"`
	Friction   analyzer.FrictionSummary `json:"friction"`
	GapCount   int                      `json:"gap_count"`
	Critical   int                      `json:"critical"`
	Warnings   int                      `json:"warnings"`
	InfoCount  int                      `json:"info"`
}

func runGaps(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Load all data sources.
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
		settings = nil
	}

	commands, err := claude.ListCommands(cfg.ClaudeHome)
	if err != nil {
		commands = nil
	}

	// Run friction analysis.
	friction := analyzer.AnalyzeFriction(facets, cfg.Friction.RecurringThreshold)

	// Collect gaps.
	var gaps []gap

	// 1. CLAUDE.md gaps: projects with sessions but no CLAUDE.md.
	claudeMDGaps := findClaudeMDGaps(sessions, cfg.ScanPaths)
	gaps = append(gaps, claudeMDGaps...)

	// 2. Recurring friction.
	frictionGaps := findRecurringFrictionGaps(friction, facets)
	gaps = append(gaps, frictionGaps...)

	// 3. Missing hooks.
	hookGaps := findMissingHookGaps(settings)
	gaps = append(gaps, hookGaps...)

	// 4. Unused skills.
	skillGaps := findUnusedSkillGaps(commands)
	gaps = append(gaps, skillGaps...)

	// 5. Project-specific friction.
	projectFrictionGaps := findProjectFrictionGaps(facets, sessions)
	gaps = append(gaps, projectFrictionGaps...)

	// 6. CLAUDE.md quality gaps.
	claudeMDQualityGaps := findClaudeMDQualityGaps(cfg.ScanPaths, facets)
	gaps = append(gaps, claudeMDQualityGaps...)

	// 7. Stale friction gaps.
	staleFrictionGaps := findStaleFrictionGaps(facets, sessions)
	gaps = append(gaps, staleFrictionGaps...)

	// 8. Tool anomaly gaps.
	toolAnomalyGaps := findToolAnomalyGaps(sessions, cfg.ScanPaths)
	gaps = append(gaps, toolAnomalyGaps...)

	// Count severities.
	var critical, warnings, infoCount int
	for _, g := range gaps {
		switch g.Severity {
		case "critical":
			critical++
		case "warning":
			warnings++
		case "info":
			infoCount++
		}
	}

	// JSON output mode.
	if flagJSON {
		out := gapsOutput{
			Gaps:      gaps,
			Friction:  friction,
			GapCount:  len(gaps),
			Critical:  critical,
			Warnings:  warnings,
			InfoCount: infoCount,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Render styled output.
	fmt.Println(output.Section("Gap Analysis"))
	fmt.Printf(" Found %d gaps: %s critical, %s warnings, %s info\n\n",
		len(gaps),
		output.StyleError.Render(fmt.Sprintf("%d", critical)),
		output.StyleWarning.Render(fmt.Sprintf("%d", warnings)),
		output.StyleMuted.Render(fmt.Sprintf("%d", infoCount)))

	renderGapsByCategory(gaps)

	// Friction summary.
	if friction.TotalFrictionEvents > 0 {
		fmt.Println(output.Section("Friction Summary"))
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Total friction events"),
			output.StyleValue.Render(fmt.Sprintf("%d", friction.TotalFrictionEvents)))
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("Sessions with friction"),
			output.StyleValue.Render(fmt.Sprintf("%d/%d", friction.SessionsWithFriction, friction.TotalSessions)))

		if len(friction.FrictionByType) > 0 {
			fmt.Printf("\n %s\n", output.StyleMuted.Render("Friction by type:"))
			sorted := sortMapByValue(friction.FrictionByType)
			for _, kv := range sorted {
				fmt.Printf("   %s %s\n",
					output.StyleLabel.Render(kv.key),
					output.StyleValue.Render(fmt.Sprintf("%d", kv.value)))
			}
		}
		fmt.Println()
	}

	return nil
}

// findClaudeMDGaps identifies projects with sessions but no CLAUDE.md.
func findClaudeMDGaps(sessions []claude.SessionMeta, scanPaths []string) []gap {
	// Collect unique project paths from sessions.
	projectPaths := make(map[string]int)
	for _, s := range sessions {
		if s.ProjectPath != "" {
			projectPaths[s.ProjectPath]++
		}
	}

	var gaps []gap
	for project, sessionCount := range projectPaths {
		claudeMDPath := filepath.Join(project, "CLAUDE.md")
		if _, err := os.Stat(claudeMDPath); os.IsNotExist(err) {
			gaps = append(gaps, gap{
				Severity: "critical",
				Category: "claude_md",
				Title:    "Missing CLAUDE.md",
				Detail:   fmt.Sprintf("%s has %d sessions but no CLAUDE.md", filepath.Base(project), sessionCount),
				Project:  project,
			})
		}
	}

	// Sort by session count descending for stable output.
	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].Detail > gaps[j].Detail
	})

	return gaps
}

// findRecurringFrictionGaps flags friction types appearing in >30% of sessions.
func findRecurringFrictionGaps(friction analyzer.FrictionSummary, facets []claude.SessionFacet) []gap {
	var gaps []gap

	for _, frictionType := range friction.RecurringFriction {
		count := friction.FrictionByType[frictionType]
		pct := 0.0
		if friction.TotalSessions > 0 {
			// Calculate the percentage of sessions with this friction type.
			typeSessionCount := 0
			for _, f := range facets {
				if _, ok := f.FrictionCounts[frictionType]; ok {
					typeSessionCount++
				}
			}
			pct = float64(typeSessionCount) / float64(friction.TotalSessions) * 100
		}

		gaps = append(gaps, gap{
			Severity: "warning",
			Category: "friction",
			Title:    fmt.Sprintf("Recurring friction: %s", frictionType),
			Detail:   fmt.Sprintf("Appears in %.0f%% of sessions (%d total occurrences)", pct, count),
		})
	}

	return gaps
}

// recommendedHooks lists hook events that are recommended for productive workflows.
var recommendedHooks = []string{
	"PreToolUse",
	"PostToolUse",
	"SessionStart",
	"SessionEnd",
}

// findMissingHookGaps checks settings for recommended hooks.
func findMissingHookGaps(settings *claude.GlobalSettings) []gap {
	var gaps []gap

	if settings == nil {
		gaps = append(gaps, gap{
			Severity: "warning",
			Category: "hooks",
			Title:    "No settings.json found",
			Detail:   "Cannot analyze hook configuration without settings.json",
		})
		return gaps
	}

	for _, hook := range recommendedHooks {
		if _, ok := settings.Hooks[hook]; !ok {
			gaps = append(gaps, gap{
				Severity: "info",
				Category: "hooks",
				Title:    fmt.Sprintf("No %s hook configured", hook),
				Detail:   fmt.Sprintf("Consider adding a %s hook for automation", hook),
			})
		}
	}

	return gaps
}

// findUnusedSkillGaps lists custom command files (skills).
func findUnusedSkillGaps(commands []claude.CommandFile) []gap {
	var gaps []gap

	if len(commands) == 0 {
		gaps = append(gaps, gap{
			Severity: "info",
			Category: "skills",
			Title:    "No custom commands defined",
			Detail:   "Custom slash commands in ~/.claude/commands/ can automate common tasks",
		})
		return gaps
	}

	// List available commands as informational.
	var names []string
	for _, cmd := range commands {
		names = append(names, cmd.Name)
	}
	sort.Strings(names)

	gaps = append(gaps, gap{
		Severity: "info",
		Category: "skills",
		Title:    fmt.Sprintf("%d custom commands available", len(commands)),
		Detail:   fmt.Sprintf("Commands: %s", strings.Join(names, ", ")),
	})

	return gaps
}

// findProjectFrictionGaps cross-references facets with sessions to identify
// projects with disproportionate friction.
func findProjectFrictionGaps(facets []claude.SessionFacet, sessions []claude.SessionMeta) []gap {
	// Build a session-to-project mapping.
	sessionProject := make(map[string]string)
	for _, s := range sessions {
		sessionProject[s.SessionID] = s.ProjectPath
	}

	// Aggregate friction by project.
	projectFriction := make(map[string]int)
	projectSessions := make(map[string]int)

	for _, f := range facets {
		project := sessionProject[f.SessionID]
		if project == "" {
			continue
		}
		projectSessions[project]++
		for _, count := range f.FrictionCounts {
			projectFriction[project] += count
		}
	}

	// Calculate average friction per session across all projects.
	totalFriction := 0
	totalSessions := 0
	for project, friction := range projectFriction {
		totalFriction += friction
		totalSessions += projectSessions[project]
	}

	if totalSessions == 0 {
		return nil
	}

	avgFriction := float64(totalFriction) / float64(totalSessions)

	// Flag projects with friction significantly above average.
	var gaps []gap
	for project, friction := range projectFriction {
		sessions := projectSessions[project]
		if sessions == 0 {
			continue
		}
		projectAvg := float64(friction) / float64(sessions)
		if projectAvg > avgFriction*2 && friction > 2 {
			gaps = append(gaps, gap{
				Severity: "warning",
				Category: "project_friction",
				Title:    fmt.Sprintf("High friction: %s", filepath.Base(project)),
				Detail:   fmt.Sprintf("%.1f friction/session vs %.1f average (%d sessions)", projectAvg, avgFriction, sessions),
				Project:  project,
			})
		}
	}

	return gaps
}

// findClaudeMDQualityGaps runs the CLAUDE.md effectiveness analyzer and flags
// projects with quality scores below 50.
func findClaudeMDQualityGaps(scanPaths []string, facets []claude.SessionFacet) []gap {
	projects, err := scanner.DiscoverProjects(scanPaths)
	if err != nil {
		log.Printf("Warning: could not discover projects for CLAUDE.md quality analysis: %v", err)
		return nil
	}

	analysis := analyzer.AnalyzeClaudeMDEffectiveness(projects, facets)

	var gaps []gap
	for _, proj := range analysis.Projects {
		if proj.QualityScore < 50 {
			missing := strings.Join(proj.MissingSections, ", ")
			if missing == "" {
				missing = "none detected"
			}
			gaps = append(gaps, gap{
				Severity: "warning",
				Category: "claude_md_quality",
				Title:    fmt.Sprintf("Low CLAUDE.md quality: %s (score %d/100)", proj.ProjectName, proj.QualityScore),
				Detail:   fmt.Sprintf("Missing sections: %s", missing),
				Project:  proj.ProjectPath,
			})
		}
	}

	return gaps
}

// findStaleFrictionGaps flags friction types that have persisted for 3+ consecutive
// weeks without improvement.
func findStaleFrictionGaps(facets []claude.SessionFacet, sessions []claude.SessionMeta) []gap {
	persistence := analyzer.AnalyzeFrictionPersistence(facets, sessions)

	var gaps []gap
	for _, p := range persistence.Patterns {
		if p.Stale {
			gaps = append(gaps, gap{
				Severity: "critical",
				Category: "stale_friction",
				Title:    fmt.Sprintf("Stale friction: %s", p.FrictionType),
				Detail:   fmt.Sprintf("%d consecutive weeks without improvement (appeared in %d sessions)", p.ConsecutiveWeeks, p.OccurrenceCount),
			})
		}
	}

	return gaps
}

// findToolAnomalyGaps runs the tool usage analyzer and flags detected anomalies.
func findToolAnomalyGaps(sessions []claude.SessionMeta, scanPaths []string) []gap {
	projects, err := scanner.DiscoverProjects(scanPaths)
	if err != nil {
		log.Printf("Warning: could not discover projects for tool anomaly analysis: %v", err)
		return nil
	}

	toolAnalysis := analyzer.AnalyzeToolUsage(sessions, projects)

	var gaps []gap
	for _, anomaly := range toolAnalysis.Anomalies {
		gaps = append(gaps, gap{
			Severity: "warning",
			Category: "tool_anomaly",
			Title:    fmt.Sprintf("Tool anomaly: %s (%s)", anomaly.ProjectName, anomaly.Tool),
			Detail:   anomaly.Message,
		})
	}

	return gaps
}

// severityEmoji returns the emoji indicator for a severity level.
func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "\U0001F534" // Red circle
	case "warning":
		return "\U0001F7E1" // Yellow circle
	case "info":
		return "\U0001F7E2" // Green circle
	default:
		return " "
	}
}

// renderGapsByCategory renders gaps grouped by category.
func renderGapsByCategory(gaps []gap) {
	// Group by category.
	categories := make(map[string][]gap)
	var categoryOrder []string

	for _, g := range gaps {
		if _, exists := categories[g.Category]; !exists {
			categoryOrder = append(categoryOrder, g.Category)
		}
		categories[g.Category] = append(categories[g.Category], g)
	}

	for _, cat := range categoryOrder {
		catGaps := categories[cat]
		fmt.Printf(" %s\n", output.StyleBold.Render(categoryLabel(cat)))

		for _, g := range catGaps {
			emoji := severityEmoji(g.Severity)
			fmt.Printf("  %s %s\n", emoji, g.Title)
			fmt.Printf("    %s\n", output.StyleMuted.Render(g.Detail))
		}
		fmt.Println()
	}
}

// categoryLabel returns a human-readable label for a gap category.
func categoryLabel(cat string) string {
	switch cat {
	case "claude_md":
		return "CLAUDE.md Gaps"
	case "claude_md_quality":
		return "CLAUDE.md Quality"
	case "friction":
		return "Recurring Friction"
	case "stale_friction":
		return "Stale Friction"
	case "hooks":
		return "Hook Configuration"
	case "skills":
		return "Custom Commands"
	case "project_friction":
		return "Project-Specific Friction"
	case "tool_anomaly":
		return "Tool Anomalies"
	default:
		return strings.ReplaceAll(cat, "_", " ")
	}
}
