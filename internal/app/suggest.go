package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
	"github.com/blackwell-systems/claudewatch/internal/suggest"
	"github.com/spf13/cobra"
)

var (
	suggestLimit    int
	suggestCategory string
	suggestJSON     bool
	suggestProject  string
)

var suggestCmd = &cobra.Command{
	Use:   "suggest",
	Short: "Generate ranked improvement recommendations",
	Long: `Analyze projects, sessions, and configuration to generate actionable,
ranked improvement recommendations. Suggestions are scored by impact and
sorted from highest to lowest.`,
	RunE: runSuggest,
}

func init() {
	suggestCmd.Flags().IntVar(&suggestLimit, "limit", 10, "Maximum number of suggestions to show")
	suggestCmd.Flags().StringVar(&suggestCategory, "category", "", "Filter by category (configuration, friction, quality, adoption, agents, custom_metrics)")
	suggestCmd.Flags().BoolVar(&suggestJSON, "json", false, "Output as JSON")
	suggestCmd.Flags().StringVar(&suggestProject, "project", "", "Filter suggestions for a specific project")
	rootCmd.AddCommand(suggestCmd)
}

func runSuggest(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Build the analysis context from all data sources.
	ctx, err := buildAnalysisContext(cfg)
	if err != nil {
		return fmt.Errorf("building analysis context: %w", err)
	}

	// Run the suggest engine.
	engine := suggest.NewEngine()
	suggestions := engine.Run(ctx)

	// Filter by category if specified.
	if suggestCategory != "" {
		suggestions = filterByCategory(suggestions, suggestCategory)
	}

	// Filter by project if specified.
	if suggestProject != "" {
		suggestions = filterByProject(suggestions, suggestProject)
	}

	// Apply limit.
	if suggestLimit > 0 && len(suggestions) > suggestLimit {
		suggestions = suggestions[:suggestLimit]
	}

	if suggestJSON || flagJSON {
		return outputSuggestJSON(suggestions)
	}

	renderSuggestions(suggestions)
	return nil
}

// buildAnalysisContext loads all data sources and constructs the AnalysisContext
// needed by the suggest engine.
func buildAnalysisContext(cfg *config.Config) (*suggest.AnalysisContext, error) {
	// Discover projects.
	projects, err := scanner.DiscoverProjects(cfg.ScanPaths)
	if err != nil {
		return nil, fmt.Errorf("discovering projects: %w", err)
	}

	// Parse session metadata.
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing session meta: %w", err)
	}

	// Parse facets.
	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing facets: %w", err)
	}

	// Parse settings.
	settings, err := claude.ParseSettings(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}

	// Parse commands.
	commands, err := claude.ListCommands(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("listing commands: %w", err)
	}

	// Parse plugins.
	plugins, err := claude.ParsePlugins(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing plugins: %w", err)
	}

	// Parse agent tasks.
	agentTasks, err := claude.ParseAgentTasks()
	if err != nil {
		// Agent tasks are ephemeral; non-fatal if missing.
		agentTasks = nil
	}

	// Compute average tool errors across all sessions.
	var totalToolErrors int
	for _, s := range sessions {
		totalToolErrors += s.ToolErrors
	}
	avgToolErrors := 0.0
	if len(sessions) > 0 {
		avgToolErrors = float64(totalToolErrors) / float64(len(sessions))
	}

	// Build a map from session ID to project path for cross-referencing.
	sessionProject := make(map[string]string)
	for _, s := range sessions {
		sessionProject[s.SessionID] = s.ProjectPath
	}

	// Compute recurring friction types (>30% of faceted sessions).
	frictionSessionCount := make(map[string]int)
	for _, f := range facets {
		for frictionType := range f.FrictionCounts {
			frictionSessionCount[frictionType]++
		}
	}
	var recurringFriction []string
	if len(facets) > 0 {
		threshold := cfg.Friction.RecurringThreshold
		for frictionType, count := range frictionSessionCount {
			if float64(count)/float64(len(facets)) > threshold {
				recurringFriction = append(recurringFriction, frictionType)
			}
		}
	}

	// Compute hook count.
	hookCount := 0
	if settings != nil {
		for _, groups := range settings.Hooks {
			hookCount += len(groups)
		}
	}

	// Compute plugin count.
	pluginCount := 0
	for _, p := range plugins {
		if p.Enabled {
			pluginCount++
		}
	}

	// Compute agent stats.
	agentTypeStats := make(map[string]float64)
	agentOverallSuccess := 0.0
	if len(agentTasks) > 0 {
		typeCount := make(map[string]int)
		typeSuccess := make(map[string]int)
		totalSuccess := 0
		for _, task := range agentTasks {
			typeCount[task.AgentType]++
			if task.Status == "completed" {
				typeSuccess[task.AgentType]++
				totalSuccess++
			}
		}
		agentOverallSuccess = float64(totalSuccess) / float64(len(agentTasks))
		for agentType, count := range typeCount {
			agentTypeStats[agentType] = float64(typeSuccess[agentType]) / float64(count)
		}
	}

	// Build project contexts.
	projectContexts := make([]suggest.ProjectContext, len(projects))
	for i, p := range projects {
		// Count sessions for this project.
		var projectToolErrors, projectInterruptions, projectAgents, projectSequential int
		hasFacets := false
		for _, s := range sessions {
			if normalizePath(s.ProjectPath) == normalizePath(p.Path) {
				projectToolErrors += s.ToolErrors
				projectInterruptions += s.UserInterruptions
			}
		}
		for _, f := range facets {
			sid := f.SessionID
			if normalizePath(sessionProject[sid]) == normalizePath(p.Path) {
				hasFacets = true
			}
		}
		for _, task := range agentTasks {
			if normalizePath(sessionProject[task.SessionID]) == normalizePath(p.Path) {
				projectAgents++
				if !task.Background {
					projectSequential++
				}
			}
		}

		projectContexts[i] = suggest.ProjectContext{
			Path:            p.Path,
			Name:            p.Name,
			HasClaudeMD:     p.HasClaudeMD,
			SessionCount:    p.SessionCount,
			ToolErrors:      projectToolErrors,
			Interruptions:   projectInterruptions,
			Score:           p.Score,
			HasFacets:       hasFacets,
			AgentCount:      projectAgents,
			SequentialCount: projectSequential,
		}
	}

	// Custom metric trends: placeholder for now (populated by track command).
	customMetricTrends := make(map[string]string)

	ctx := &suggest.AnalysisContext{
		Projects:           projectContexts,
		TotalSessions:      len(sessions),
		AvgToolErrors:      avgToolErrors,
		RecurringFriction:  recurringFriction,
		HookCount:          hookCount,
		CommandCount:        len(commands),
		PluginCount:        pluginCount,
		AgentSuccessRate:   agentOverallSuccess,
		AgentTypeStats:     agentTypeStats,
		CustomMetricTrends: customMetricTrends,
	}

	return ctx, nil
}

func filterByCategory(suggestions []suggest.Suggestion, category string) []suggest.Suggestion {
	var filtered []suggest.Suggestion
	for _, s := range suggestions {
		if s.Category == category {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func filterByProject(suggestions []suggest.Suggestion, project string) []suggest.Suggestion {
	var filtered []suggest.Suggestion
	for _, s := range suggestions {
		// Include suggestions whose title or description mentions the project name,
		// and category-wide suggestions (not project-specific).
		if strings.Contains(s.Title, project) || strings.Contains(s.Description, project) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func outputSuggestJSON(suggestions []suggest.Suggestion) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(suggestions)
}

func renderSuggestions(suggestions []suggest.Suggestion) {
	if len(suggestions) == 0 {
		fmt.Println(output.Section("Suggestions"))
		fmt.Println()
		fmt.Println(" No suggestions. Your workflow looks good!")
		return
	}

	fmt.Println(output.Section("Improvement Suggestions"))
	fmt.Println()

	for i, s := range suggestions {
		priorityLabel := priorityToLabel(s.Priority)
		priorityStyled := stylePriority(s.Priority, priorityLabel)

		fmt.Printf(" #%d %s %s\n", i+1, priorityStyled, output.StyleBold.Render(s.Title))
		fmt.Printf("    Impact: %.1f  |  Category: %s\n", s.ImpactScore, s.Category)
		fmt.Printf("    %s\n", s.Description)
		fmt.Println()
	}
}

func priorityToLabel(priority int) string {
	switch priority {
	case suggest.PriorityCritical:
		return "[CRITICAL]"
	case suggest.PriorityHigh:
		return "[HIGH]"
	case suggest.PriorityMedium:
		return "[MEDIUM]"
	case suggest.PriorityLow:
		return "[LOW]"
	default:
		return "[UNKNOWN]"
	}
}

func stylePriority(priority int, label string) string {
	switch priority {
	case suggest.PriorityCritical, suggest.PriorityHigh:
		return output.StyleError.Render(label)
	case suggest.PriorityMedium:
		return output.StyleWarning.Render(label)
	default:
		return output.StyleMuted.Render(label)
	}
}

// normalizePath removes trailing slashes for consistent path comparison.
func normalizePath(p string) string {
	return strings.TrimRight(p, "/")
}
