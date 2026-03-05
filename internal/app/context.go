package app

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/client"
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

	project, _ := cmd.Flags().GetString("project")
	limit, _ := cmd.Flags().GetInt("limit")

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Create MCP client and fetch from all sources
	mcpClient := client.NewMCPClient()
	ctx := stdcontext.Background()
	rawResults, errors := client.FetchAllSources(ctx, mcpClient, query, project, limit)

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
}

// parseSourceResults parses raw JSON from a source into ContextItems.
func parseSourceResults(source string, rawJSON []byte) ([]context.ContextItem, error) {
	// Handle empty results (e.g., from local tool placeholders)
	if len(rawJSON) == 0 || string(rawJSON) == "{}" {
		return []context.ContextItem{}, nil
	}

	switch source {
	case "memory":
		return parseMemoryResults(rawJSON)
	case "commit":
		return parseCommitResults(rawJSON)
	case "task_history":
		return parseTaskHistoryResults(rawJSON)
	case "transcript":
		return parseTranscriptResults(rawJSON)
	default:
		return nil, fmt.Errorf("unknown source: %s", source)
	}
}

// parseMemoryResults parses commitmux_search_memory response.
func parseMemoryResults(rawJSON []byte) ([]context.ContextItem, error) {
	// commitmux returns array of results with fields: path, content, distance, timestamp
	var response struct {
		Results []struct {
			Path      string    `json:"path"`
			Content   string    `json:"content"`
			Distance  float64   `json:"distance"`
			Timestamp time.Time `json:"timestamp"`
		} `json:"results"`
	}

	if err := json.Unmarshal(rawJSON, &response); err != nil {
		return nil, fmt.Errorf("failed to parse memory results: %w", err)
	}

	items := make([]context.ContextItem, 0, len(response.Results))
	for _, r := range response.Results {
		items = append(items, context.ContextItem{
			Source:    context.SourceMemory,
			Title:     fmt.Sprintf("memory: %s", r.Path),
			Snippet:   r.Content,
			Timestamp: r.Timestamp,
			Metadata: map[string]string{
				"path": r.Path,
			},
			Score: 1.0 - r.Distance, // Convert distance to score (lower distance = higher score)
		})
	}

	return items, nil
}

// parseCommitResults parses commitmux_search_semantic response.
func parseCommitResults(rawJSON []byte) ([]context.ContextItem, error) {
	// commitmux returns array of results with fields: sha, message, author, timestamp, distance
	var response struct {
		Results []struct {
			SHA       string    `json:"sha"`
			Message   string    `json:"message"`
			Author    string    `json:"author"`
			Timestamp time.Time `json:"timestamp"`
			Distance  float64   `json:"distance"`
			Repo      string    `json:"repo"`
		} `json:"results"`
	}

	if err := json.Unmarshal(rawJSON, &response); err != nil {
		return nil, fmt.Errorf("failed to parse commit results: %w", err)
	}

	items := make([]context.ContextItem, 0, len(response.Results))
	for _, r := range response.Results {
		items = append(items, context.ContextItem{
			Source:    context.SourceCommit,
			Title:     fmt.Sprintf("commit: %s", r.SHA[:8]),
			Snippet:   r.Message,
			Timestamp: r.Timestamp,
			Metadata: map[string]string{
				"sha":    r.SHA,
				"author": r.Author,
				"repo":   r.Repo,
			},
			Score: 1.0 - r.Distance,
		})
	}

	return items, nil
}

// parseTaskHistoryResults parses get_task_history response.
func parseTaskHistoryResults(rawJSON []byte) ([]context.ContextItem, error) {
	// Local task history returns array of tasks
	var response struct {
		Tasks []struct {
			Description string    `json:"description"`
			Status      string    `json:"status"`
			Timestamp   time.Time `json:"timestamp"`
			SessionID   string    `json:"session_id"`
			Score       float64   `json:"score,omitempty"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal(rawJSON, &response); err != nil {
		return nil, fmt.Errorf("failed to parse task history results: %w", err)
	}

	items := make([]context.ContextItem, 0, len(response.Tasks))
	for _, t := range response.Tasks {
		score := t.Score
		if score == 0 {
			score = 0.5 // Default score for keyword match
		}

		items = append(items, context.ContextItem{
			Source:    context.SourceTaskHistory,
			Title:     fmt.Sprintf("task: %s", t.Description),
			Snippet:   t.Description,
			Timestamp: t.Timestamp,
			Metadata: map[string]string{
				"status":     t.Status,
				"session_id": t.SessionID,
			},
			Score: score,
		})
	}

	return items, nil
}

// parseTranscriptResults parses search_transcripts response.
func parseTranscriptResults(rawJSON []byte) ([]context.ContextItem, error) {
	// Local transcript search returns array of results
	var response struct {
		Results []struct {
			SessionID string    `json:"session_id"`
			EntryType string    `json:"entry_type"`
			Timestamp time.Time `json:"timestamp"`
			Snippet   string    `json:"snippet"`
			Rank      float64   `json:"rank,omitempty"`
		} `json:"results"`
	}

	if err := json.Unmarshal(rawJSON, &response); err != nil {
		return nil, fmt.Errorf("failed to parse transcript results: %w", err)
	}

	items := make([]context.ContextItem, 0, len(response.Results))
	for _, r := range response.Results {
		score := r.Rank
		if score == 0 {
			score = 0.5 // Default score
		}

		items = append(items, context.ContextItem{
			Source:    context.SourceTranscript,
			Title:     fmt.Sprintf("transcript: %s", r.SessionID[:12]),
			Snippet:   r.Snippet,
			Timestamp: r.Timestamp,
			Metadata: map[string]string{
				"session_id": r.SessionID,
				"entry_type": r.EntryType,
			},
			Score: score,
		})
	}

	return items, nil
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
