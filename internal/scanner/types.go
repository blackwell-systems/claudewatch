// Package scanner provides project discovery and readiness scoring.
package scanner

// Project represents a discovered project with all metadata needed for scoring.
type Project struct {
	// Path is the absolute filesystem path to the project root.
	Path string `json:"path"`

	// Name is the directory name of the project.
	Name string `json:"name"`

	// HasClaudeMD indicates whether a CLAUDE.md file exists in the project root.
	HasClaudeMD bool `json:"has_claude_md"`

	// ClaudeMDSize is the size of the CLAUDE.md file in bytes (0 if absent).
	ClaudeMDSize int64 `json:"claude_md_size"`

	// HasDotClaude indicates whether a .claude/ directory exists.
	HasDotClaude bool `json:"has_dot_claude"`

	// HasLocalSettings indicates whether .claude/settings.local.json exists.
	HasLocalSettings bool `json:"has_local_settings"`

	// PrimaryLanguage is the most-used language detected from session data.
	PrimaryLanguage string `json:"primary_language,omitempty"`

	// CommitsLast30Days is the number of git commits in the last 30 days.
	CommitsLast30Days int `json:"commits_last_30_days"`

	// SessionCount is the number of Claude sessions associated with this project.
	SessionCount int `json:"session_count"`

	// LastSessionDate is the start time of the most recent session.
	LastSessionDate string `json:"last_session_date,omitempty"`

	// HasFacets indicates whether facet data exists for this project's sessions.
	HasFacets bool `json:"has_facets"`

	// HasGit indicates whether the project is a git repository.
	HasGit bool `json:"has_git"`

	// Score is the computed readiness score (0-100).
	Score float64 `json:"score"`
}
