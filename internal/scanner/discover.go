package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverProjects walks each provided path looking for directories that
// contain a .git/ directory (i.e., Git repositories). For each discovered
// project it checks for CLAUDE.md, .claude/ directory, .claude/settings.local.json,
// detects primary language, and counts recent git commits.
func DiscoverProjects(paths []string) ([]Project, error) {
	var projects []Project
	seen := make(map[string]bool)

	for _, root := range paths {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// Skip hidden directories.
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			projectPath := filepath.Join(root, entry.Name())

			// Only include git repositories.
			gitDir := filepath.Join(projectPath, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				continue
			}

			// Deduplicate by absolute path.
			abs, err := filepath.Abs(projectPath)
			if err != nil {
				abs = projectPath
			}
			if seen[abs] {
				continue
			}
			seen[abs] = true

			p := Project{
				Path:   abs,
				Name:   entry.Name(),
				HasGit: true,
			}

			// Check CLAUDE.md.
			claudeMDPath := filepath.Join(abs, "CLAUDE.md")
			if info, err := os.Stat(claudeMDPath); err == nil {
				p.HasClaudeMD = true
				p.ClaudeMDSize = info.Size()
			}

			// Check .claude/ directory.
			dotClaudePath := filepath.Join(abs, ".claude")
			if info, err := os.Stat(dotClaudePath); err == nil && info.IsDir() {
				p.HasDotClaude = true
			}

			// Check .claude/settings.local.json.
			localSettingsPath := filepath.Join(abs, ".claude", "settings.local.json")
			if _, err := os.Stat(localSettingsPath); err == nil {
				p.HasLocalSettings = true
			}

			// Detect primary language.
			p.PrimaryLanguage = detectLanguage(abs)

			// Count recent git commits.
			p.CommitsLast30Days = countRecentCommits(abs)

			projects = append(projects, p)
		}
	}

	// Sort by name.
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, nil
}

// detectLanguage infers the primary language from the presence of
// well-known project files.
func detectLanguage(projectPath string) string {
	// Ordered by specificity: check more specific indicators first.
	indicators := []struct {
		file string
		lang string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"package.json", "JavaScript"},
		{"pyproject.toml", "Python"},
		{"setup.py", "Python"},
	}

	for _, ind := range indicators {
		if _, err := os.Stat(filepath.Join(projectPath, ind.file)); err == nil {
			return ind.lang
		}
	}
	return ""
}

// countRecentCommits runs git log to count commits in the last 30 days.
func countRecentCommits(projectPath string) int {
	cmd := exec.Command("git", "log", "--oneline", "--since=30 days ago")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	lines := strings.Split(trimmed, "\n")
	return len(lines)
}
