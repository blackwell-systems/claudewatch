package mcp

import (
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// sessionMatchesProject returns true if the session's activity includes the
// given project filter string. Matches against primary project name or any
// project in the weights list. Case-insensitive.
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights []store.ProjectWeight, filter string) bool {
	if filter == "" {
		return true
	}
	primary := sessionPrimaryProject(sessionID, projectPath, tags, weights)
	if strings.EqualFold(primary, filter) {
		return true
	}
	for _, w := range weights {
		if strings.EqualFold(w.Project, filter) {
			return true
		}
	}
	return false
}

// sessionPrimaryProject returns the primary project name for a session.
// Priority: explicit tag > highest-weight project > filepath.Base(projectPath).
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights []store.ProjectWeight) string {
	if name, ok := tags[sessionID]; ok && name != "" {
		return name
	}
	if len(weights) > 0 {
		best := weights[0]
		for _, w := range weights[1:] {
			if w.Weight > best.Weight {
				best = w
			}
		}
		if best.Project != "" {
			return best.Project
		}
	}
	return filepath.Base(projectPath)
}
