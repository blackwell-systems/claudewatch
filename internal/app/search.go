package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var (
	searchFlagLimit int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search over indexed session transcripts",
	Long: `Search indexed session transcripts using full-text search (FTS5).
On first use, transcripts are indexed automatically. Use standard SQLite
FTS5 query syntax for advanced searches.

Examples:
  claudewatch search "anomaly detection"
  claudewatch search refactor
  claudewatch search "NOT error"
  claudewatch search --limit 5 "doctor"`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVar(&searchFlagLimit, "limit", 20, "Maximum number of results to return")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Auto-index if the index is empty.
	count, _, statusErr := db.TranscriptIndexStatus()
	if statusErr != nil {
		return fmt.Errorf("checking transcript index status: %w", statusErr)
	}
	if count == 0 {
		if !flagJSON {
			fmt.Fprintln(os.Stderr, "Indexing transcripts…")
		}
		if _, indexErr := db.IndexTranscripts(cfg.ClaudeHome, false); indexErr != nil {
			return fmt.Errorf("indexing transcripts: %w", indexErr)
		}
	}

	results, err := db.SearchTranscripts(query, searchFlagLimit)
	if err != nil {
		return fmt.Errorf("searching transcripts: %w", err)
	}

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	renderSearchResults(results, query)
	return nil
}

func renderSearchResults(results []store.TranscriptSearchResult, query string) {
	fmt.Println(output.Section("Search Results"))
	fmt.Println()

	if len(results) == 0 {
		fmt.Printf(" %s\n\n", output.StyleMuted.Render(fmt.Sprintf("No results found for %q", query)))
		return
	}

	fmt.Printf(" %s  query: %s\n\n",
		output.StyleMuted.Render(fmt.Sprintf("%d result(s)", len(results))),
		output.StyleBold.Render(query))

	tbl := output.NewTable("Session", "Type", "Timestamp", "Snippet")

	for _, r := range results {
		sessionShort := r.SessionID
		if len(sessionShort) > 12 {
			sessionShort = sessionShort[:12]
		}

		ts := r.Timestamp
		if len(ts) > 16 {
			ts = ts[:16]
		}

		snippet := r.Snippet
		if len(snippet) > 60 {
			snippet = snippet[:60] + "…"
		}

		tbl.AddRow(sessionShort, r.EntryType, ts, snippet)
	}

	tbl.Print()
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --limit to show more results, --json for machine output"))
	fmt.Println()
}
