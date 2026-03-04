package mcp

import (
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// sessionMatchesProject returns true if the session's activity includes the
// given project filter string. Matches against primary project name or any
// project in the weights list.
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights []store.ProjectWeight, filter string) bool {
	panic("not implemented")
}

// sessionPrimaryProject returns the primary project name for a session,
// preferring the highest-weight project from weights if available, falling
// back to resolveProjectName.
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights []store.ProjectWeight) string {
	panic("not implemented")
}
