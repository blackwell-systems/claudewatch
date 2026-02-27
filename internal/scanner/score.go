package scanner

import (
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ComputeReadiness calculates a 0-100 readiness score for a project based on
// the scoring algorithm defined in the plan (section 6).
//
// Scoring breakdown:
//   - CLAUDE.md exists:    30 points
//   - CLAUDE.md quality:   0-10 points (based on file size)
//   - .claude/ directory:  10 points
//   - Local settings:      5 points
//   - Session history:     0-15 points (scaled by recency)
//   - Facets coverage:     10 points
//   - Active development:  0-10 points (based on commits in last 30 days)
//   - Hook adoption:       5 points
//   - Plugin usage:        5 points
func ComputeReadiness(p *Project, sessions []claude.SessionMeta, facets []claude.SessionFacet, settings *claude.GlobalSettings) float64 {
	score := 0.0

	// CLAUDE.md exists: 30 points.
	if p.HasClaudeMD {
		score += 30
	}

	// CLAUDE.md quality: 0-10 based on file size.
	if p.ClaudeMDSize > 500 {
		score += 10
	} else if p.ClaudeMDSize > 100 {
		score += 5
	}

	// .claude/ directory: 10 points.
	if p.HasDotClaude {
		score += 10
	}

	// Local settings: 5 points.
	if p.HasLocalSettings {
		score += 5
	}

	// Session history: 15 points scaled by recency.
	projectSessions := filterByProject(sessions, p.Path)
	if len(projectSessions) > 0 {
		// Use the most recent session's start time for recency weighting.
		mostRecent := projectSessions[len(projectSessions)-1]
		score += 15 * recencyWeight(mostRecent.StartTime)
	}

	// Facets coverage: 10 points if any facet data exists for this project.
	if len(FilterFacetsByProject(facets, sessions, p.Path)) > 0 {
		score += 10
	}

	// Active development: 0-10 based on commits in last 30 days.
	switch {
	case p.CommitsLast30Days > 20:
		score += 10
	case p.CommitsLast30Days > 5:
		score += 5
	case p.CommitsLast30Days > 0:
		score += 2
	}

	// Hook adoption: 5 points if global hooks are configured.
	if settings != nil && len(settings.Hooks) > 0 {
		score += 5
	}

	// Plugin usage: 5 points if a relevant plugin is enabled.
	if settings != nil && hasRelevantPlugin(settings, p.PrimaryLanguage) {
		score += 5
	}

	return score
}

// recencyWeight returns a linear decay weight from 1.0 (today) to 0.0 (30+ days ago).
func recencyWeight(startTime string) float64 {
	if startTime == "" {
		return 0
	}

	t, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		// Try parsing date-only format as fallback.
		t, err = time.Parse("2006-01-02", startTime)
		if err != nil {
			return 0
		}
	}

	daysSince := time.Since(t).Hours() / 24
	if daysSince <= 0 {
		return 1.0
	}
	if daysSince >= 30 {
		return 0.0
	}
	return 1.0 - (daysSince / 30.0)
}

// filterByProject returns sessions whose ProjectPath matches the given path,
// sorted by StartTime ascending so the last element is the most recent.
func filterByProject(sessions []claude.SessionMeta, projectPath string) []claude.SessionMeta {
	var result []claude.SessionMeta
	for _, s := range sessions {
		if claude.NormalizePath(s.ProjectPath) == claude.NormalizePath(projectPath) {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime < result[j].StartTime
	})
	return result
}

// FilterFacetsByProject returns facets whose associated session belongs to the
// given project. It cross-references facets with sessions via SessionID.
func FilterFacetsByProject(facets []claude.SessionFacet, sessions []claude.SessionMeta, projectPath string) []claude.SessionFacet {
	// Build a set of session IDs for this project.
	projectSessionIDs := make(map[string]bool)
	for _, s := range sessions {
		if claude.NormalizePath(s.ProjectPath) == claude.NormalizePath(projectPath) {
			projectSessionIDs[s.SessionID] = true
		}
	}

	var result []claude.SessionFacet
	for _, f := range facets {
		if projectSessionIDs[f.SessionID] {
			result = append(result, f)
		}
	}
	return result
}

// hasRelevantPlugin checks whether any enabled plugin is relevant to the
// project's primary language. This is a heuristic match based on plugin name
// containing the language name.
func hasRelevantPlugin(settings *claude.GlobalSettings, language string) bool {
	if language == "" || len(settings.EnabledPlugins) == 0 {
		return false
	}
	lang := strings.ToLower(language)
	for name, enabled := range settings.EnabledPlugins {
		if enabled && strings.Contains(strings.ToLower(name), lang) {
			return true
		}
	}
	return false
}
