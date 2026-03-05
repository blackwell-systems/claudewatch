package context

import "time"

// SourceType identifies where a context item came from.
type SourceType string

const (
	SourceMemory      SourceType = "memory"
	SourceCommit      SourceType = "commit"
	SourceTaskHistory SourceType = "task_history"
	SourceTranscript  SourceType = "transcript"
)

// ContextItem is one result from a unified context search.
type ContextItem struct {
	Source      SourceType        `json:"source"`
	Title       string            `json:"title"`              // e.g., "memory: MEMORY.md", "commit: abc123", "task: implement X"
	Snippet     string            `json:"snippet"`            // text excerpt
	Timestamp   time.Time         `json:"timestamp"`          // when the item was created/committed
	Metadata    map[string]string `json:"metadata,omitempty"` // source-specific fields (repo, sha, session_id, etc.)
	Score       float64           `json:"score"`              // relevance score (0.0-1.0, higher = better)
	ContentHash string            `json:"-"`                  // SHA-256 of normalized content (for dedup, not returned to caller)
}

// UnifiedContextResult is the MCP tool and CLI response.
type UnifiedContextResult struct {
	Query   string        `json:"query"`
	Items   []ContextItem `json:"items"`
	Count   int           `json:"count"`
	Sources []string      `json:"sources"`          // list of sources queried (e.g., ["memory", "commit", "task_history"])
	Errors  []string      `json:"errors,omitempty"` // partial failure messages
}
