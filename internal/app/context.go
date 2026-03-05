package app

import (
	"fmt"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/context"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context <query>",
	Short: "Unified context search across commits, memory, tasks, and transcripts",
	Long: `Search across multiple context sources to find relevant information:
  - Semantic memory search (via commitmux)
  - Commit history search (via commitmux)
  - Task history search (local)
  - Transcript search (local)

Results are deduplicated, ranked by relevance and recency, and presented
with source attribution.

Examples:
  claudewatch context "authentication implementation"
  claudewatch context --limit 10 "error handling"
  claudewatch context --project commitmux "MCP tools"
  claudewatch context --json "session patterns"`,
	Args: cobra.ExactArgs(1),
	RunE: runContext,
}

func init() {
	contextCmd.Flags().String("project", "", "Project name filter (optional)")
	contextCmd.Flags().Int("limit", 20, "Maximum results to return")
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	query := args[0]
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// project, _ := cmd.Flags().GetString("project")
	// limit, _ := cmd.Flags().GetInt("limit")

	if flagNoColor {
		output.SetNoColor(true)
	}

	// TODO: Once Agent A completes, replace this stub with actual parallel execution:
	// client := client.NewMCPClient()
	// rawResults, errors := client.FetchAllSources(context.Background(), query, project, limit)
	// For now, return a helpful error message.
	return fmt.Errorf("unified context search not yet available: Agent A (MCP client) pending completion")

	// The code below shows the intended implementation once Agent A is merged:
	/*
		// Parse raw JSON results into ContextItems
		var allItems []context.ContextItem
		var parseErrors []string

		for source, rawJSON := range rawResults {
			items, err := parseSourceResults(source, rawJSON)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", source, err))
				continue
			}
			allItems = append(allItems, items...)
		}

		// Add errors from parallel execution
		for _, err := range errors {
			parseErrors = append(parseErrors, err.Error())
		}

		// If all sources failed, return error
		if len(allItems) == 0 && len(parseErrors) > 0 {
			if !flagJSON {
				fmt.Fprintln(os.Stderr, "All sources failed:")
				for _, e := range parseErrors {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
			}
			return fmt.Errorf("all context sources failed")
		}

		// Deduplicate and rank
		allItems = context.DeduplicateItems(allItems)
		context.RankAndSort(allItems)

		// Apply limit
		if limit > 0 && len(allItems) > limit {
			allItems = allItems[:limit]
		}

		// Build result
		result := context.UnifiedContextResult{
			Query:   query,
			Items:   allItems,
			Count:   len(allItems),
			Sources: getSourcesList(rawResults),
			Errors:  parseErrors,
		}

		// Render output
		if flagJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		// Print warnings if some sources failed
		if len(parseErrors) > 0 {
			for _, e := range parseErrors {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", e)
			}
			fmt.Fprintln(os.Stderr)
		}

		renderContextResults(result)
		return nil
	*/
}

// parseSourceResults parses raw JSON from a source into ContextItems.
// This will be implemented once Agent A's MCP client interface is available.
func parseSourceResults(source string, rawJSON []byte) ([]context.ContextItem, error) {
	// TODO: Implement JSON parsing for each source type
	// Memory and commits: parse commitmux JSON response format
	// Task history: parse local get_task_history response
	// Transcripts: parse local search_transcripts response
	return nil, fmt.Errorf("not implemented")
}

// getSourcesList extracts source names from rawResults map.
func getSourcesList(rawResults map[string][]byte) []string {
	sources := make([]string, 0, len(rawResults))
	for source := range rawResults {
		sources = append(sources, source)
	}
	return sources
}

// renderContextResults renders UnifiedContextResult in table format.
func renderContextResults(result context.UnifiedContextResult) {
	fmt.Println(output.Section("Unified Context Search"))
	fmt.Println()

	if len(result.Items) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render(fmt.Sprintf("No results found for %q", result.Query)))
		return
	}

	fmt.Printf(" %s  query: %s\n",
		output.StyleMuted.Render(fmt.Sprintf("%d result(s) from %d source(s)", result.Count, len(result.Sources))),
		output.StyleBold.Render(result.Query))
	fmt.Printf(" %s\n\n",
		output.StyleMuted.Render(fmt.Sprintf("sources: %s", strings.Join(result.Sources, ", "))))

	tbl := output.NewTable("Source", "Title", "Timestamp", "Snippet")

	for _, item := range result.Items {
		// Format source
		sourceStr := string(item.Source)

		// Truncate title if too long
		title := item.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		// Format timestamp
		ts := item.Timestamp.Format("2006-01-02 15:04")

		// Truncate snippet
		snippet := item.Snippet
		if len(snippet) > 60 {
			snippet = snippet[:57] + "..."
		}

		tbl.AddRow(sourceStr, title, ts, snippet)
	}

	tbl.Print()
	fmt.Println()

	if len(result.Errors) > 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render(fmt.Sprintf("%d source(s) had errors", len(result.Errors))))
	}

	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --limit to show more results, --json for machine output"))
	fmt.Println()
}
