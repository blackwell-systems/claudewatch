package claude

import (
	"os"
	"path/filepath"
)

// ListProjects lists directories under ~/.claude/projects/.
// Each directory represents a project that Claude has been used with.
func ListProjects(claudeHome string) ([]ProjectDir, error) {
	dir := filepath.Join(claudeHome, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []ProjectDir
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projects = append(projects, ProjectDir{
			Path: filepath.Join(dir, entry.Name()),
			Name: entry.Name(),
		})
	}
	return projects, nil
}
