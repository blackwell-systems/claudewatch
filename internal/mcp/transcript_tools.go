package mcp

import (
	"encoding/json"
	"errors"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TranscriptSearchMCPResult wraps FTS search results for the MCP response.
type TranscriptSearchMCPResult struct {
	Count   int                           `json:"count"`
	Results []store.TranscriptSearchResult `json:"results"`
	Indexed int                           `json:"indexed_count"`
}

// addTranscriptTools registers the search_transcripts tool on s.
func addTranscriptTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "search_transcripts",
		Description: "FTS search over indexed JSONL transcripts. Returns session ID, project hash, snippet, timestamp, and rank for each match.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Full-text search query (SQLite FTS5 syntax)"},"limit":{"type":"integer","description":"Maximum results to return (default 20)"}},"required":["query"],"additionalProperties":false}`),
		Handler:     s.handleSearchTranscripts,
	})
}

// handleSearchTranscripts performs FTS search over indexed transcript entries.
// Requires the "query" arg; returns a user-friendly error if the index is empty.
func (s *Server) handleSearchTranscripts(args json.RawMessage) (any, error) {
	var params struct {
		Query *string `json:"query"`
		Limit *int    `json:"limit"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	if params.Query == nil || *params.Query == "" {
		return nil, errors.New("query is required")
	}

	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}

	db, err := store.Open(config.DBPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	// Check whether the transcript index has any entries.
	count, _, statusErr := db.TranscriptIndexStatus()
	if statusErr != nil {
		return nil, statusErr
	}
	if count == 0 {
		return nil, errors.New("transcript index is empty — run 'claudewatch search <query>' first to index transcripts, or the index will be built automatically on next search")
	}

	results, err := db.SearchTranscripts(*params.Query, limit)
	if err != nil {
		return nil, err
	}

	if results == nil {
		results = []store.TranscriptSearchResult{}
	}

	return TranscriptSearchMCPResult{
		Count:   len(results),
		Results: results,
		Indexed: count,
	}, nil
}
