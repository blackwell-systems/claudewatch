package claude

// ProjectWeight represents a single project's share of activity within a session.
type ProjectWeight struct {
	Project   string  `json:"project"`
	RepoRoot  string  `json:"repo_root"`
	Weight    float64 `json:"weight"`
	ToolCalls int     `json:"tool_calls"`
}
