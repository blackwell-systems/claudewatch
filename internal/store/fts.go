package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

const maxContentLen = 500

// extractEntryContent extracts a searchable text string from a TranscriptEntry.
// It concatenates entry.Content (non-empty for queue-operation entries) with
// any text from parsed message blocks. The result is capped at maxContentLen chars.
func extractEntryContent(entry claude.TranscriptEntry) string {
	var parts []string

	// Include the raw Content field (used by queue-operation entries).
	if entry.Content != "" {
		parts = append(parts, entry.Content)
	}

	// Try to extract text blocks from the Message field.
	if entry.Message != nil {
		// Attempt to unmarshal as a generic structure to pull out text fields.
		var msg struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &msg); err == nil {
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						parts = append(parts, block.Text)
					}
				case "tool_use":
					if block.Name != "" {
						parts = append(parts, block.Name)
					}
				}
			}
		}
	}

	combined := strings.Join(parts, " ")
	if len(combined) > maxContentLen {
		combined = combined[:maxContentLen]
	}
	return combined
}

// IndexTranscripts walks all JSONL transcript files under claudeHome/projects/ and
// upserts entries into transcript_index and transcript_index_fts. When force=false,
// already-indexed rows (by session_id + line_number) are skipped. When force=true,
// existing rows are replaced.
func (db *DB) IndexTranscripts(claudeHome string, force bool) (indexed int, err error) {
	insertVerb := "INSERT OR IGNORE"
	if force {
		insertVerb = "INSERT OR REPLACE"
	}

	insertSQL := fmt.Sprintf(`%s INTO transcript_index
		(session_id, project_hash, line_number, entry_type, content, timestamp, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, insertVerb)

	// For FTS5 with content table: we need to manually keep the FTS index in sync.
	// Use INSERT OR REPLACE into the FTS table as well.
	ftsInsertSQL := `INSERT INTO transcript_index_fts
		(rowid, session_id, project_hash, entry_type, content, timestamp)
		SELECT rowid, session_id, project_hash, entry_type, content, timestamp
		FROM transcript_index
		WHERE session_id = ? AND line_number = ?`

	indexedAt := time.Now().UTC().Format(time.RFC3339)

	// Track line numbers per session as WalkTranscriptEntries does not provide them.
	lineNumbers := make(map[string]int)

	walkErr := claude.WalkTranscriptEntries(claudeHome, func(entry claude.TranscriptEntry, sessionID string, projectHash string) {
		lineNumbers[sessionID]++
		lineNum := lineNumbers[sessionID]

		content := extractEntryContent(entry)
		if content == "" {
			// Skip entries with no extractable text.
			return
		}

		res, execErr := db.conn.Exec(insertSQL,
			sessionID,
			projectHash,
			lineNum,
			entry.Type,
			content,
			entry.Timestamp,
			indexedAt,
		)
		if execErr != nil {
			// Non-fatal: skip this entry.
			return
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			// Row already existed and force=false; skip FTS sync.
			return
		}

		// Sync new/replaced row into the FTS virtual table.
		if _, execErr = db.conn.Exec(ftsInsertSQL, sessionID, lineNum); execErr != nil {
			// Non-fatal.
			return
		}

		indexed++
	})

	if walkErr != nil {
		return indexed, fmt.Errorf("walking transcript entries: %w", walkErr)
	}

	return indexed, nil
}

// SearchTranscripts performs FTS5 full-text search over indexed transcript entries.
// query supports standard SQLite FTS5 query syntax. If limit <= 0, defaults to 20.
func (db *DB) SearchTranscripts(query string, limit int) ([]TranscriptSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	// Join FTS virtual table with backing table to get metadata.
	// snippet() generates a highlighted excerpt around the match.
	rows, err := db.conn.Query(`
		SELECT
			ti.session_id,
			ti.project_hash,
			ti.line_number,
			ti.entry_type,
			snippet(transcript_index_fts, 3, '[', ']', '...', 10) AS snippet,
			ti.timestamp,
			transcript_index_fts.rank
		FROM transcript_index_fts
		JOIN transcript_index ti
			ON transcript_index_fts.rowid = ti.rowid
		WHERE transcript_index_fts MATCH ?
		ORDER BY transcript_index_fts.rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("FTS search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []TranscriptSearchResult
	for rows.Next() {
		var r TranscriptSearchResult
		if err := rows.Scan(
			&r.SessionID,
			&r.ProjectHash,
			&r.LineNumber,
			&r.EntryType,
			&r.Snippet,
			&r.Timestamp,
			&r.Rank,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}

	return results, nil
}

// TranscriptIndexStatus returns the count of indexed entries and the most recent indexed_at timestamp.
// If no entries are indexed, lastIndexed is an empty string.
func (db *DB) TranscriptIndexStatus() (count int, lastIndexed string, err error) {
	row := db.conn.QueryRow(`
		SELECT COUNT(*), COALESCE(MAX(indexed_at), '') FROM transcript_index
	`)
	if scanErr := row.Scan(&count, &lastIndexed); scanErr != nil && scanErr != sql.ErrNoRows {
		return 0, "", fmt.Errorf("querying transcript index status: %w", scanErr)
	}
	return count, lastIndexed, nil
}
