package mcp

// projectWeightRef is a forward-reference type matching store.ProjectWeight.
// Replace with store.ProjectWeight after Wave 1 Agent B completes.
type projectWeightRef struct {
	Project   string
	RepoRoot  string
	Weight    float64
	ToolCalls int
}

// sessionMatchesProject returns true if the session is attributed to the given project.
// Priority: (1) tag override, (2) any weight entry with Project == filter (case-insensitive),
// (3) filepath.Base(projectPath) == filter (case-insensitive).
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights []projectWeightRef, filter string) bool {
	panic("not implemented")
}

// sessionPrimaryProject returns the primary project name for a session.
// Priority: (1) tag override, (2) weights[0].Project (highest weight, already sorted desc),
// (3) filepath.Base(projectPath).
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights []projectWeightRef) string {
	panic("not implemented")
}
