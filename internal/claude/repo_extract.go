package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProjectWeight represents a single project's share of activity within a session.
type ProjectWeight struct {
	Project   string  `json:"project"`
	RepoRoot  string  `json:"repo_root"`
	Weight    float64 `json:"weight"`
	ToolCalls int     `json:"tool_calls"`
}

// repoRootCache is a process-scoped cache mapping directory paths to their
// resolved git repo roots (or the directory itself if not in a repo).
var repoRootCache sync.Map

// absPathRe matches tokens that look like absolute paths: start with /,
// contain at least one more slash, and consist of path-safe characters.
var absPathRe = regexp.MustCompile(`/[a-zA-Z0-9._\-/]*[a-zA-Z0-9._\-]+/[a-zA-Z0-9._\-/]+`)

// filePathInput is used to extract path fields from various tool inputs.
type filePathInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	Command      string `json:"command"`
}

// ExtractFilePaths returns deduplicated absolute file paths found in the
// tool_use input fields of a transcript entry's assistant message content blocks.
// Inspects Read, Edit, Write, Bash, Glob, Grep, NotebookEdit tool inputs.
// Returns nil for non-assistant entries or entries with no file path tool calls.
func ExtractFilePaths(entry TranscriptEntry) []string {
	if entry.Type != "assistant" {
		return nil
	}
	if entry.Message == nil {
		return nil
	}

	var msg AssistantMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var paths []string

	addPath := func(p string) {
		if strings.HasPrefix(p, "/") {
			if _, dup := seen[p]; !dup {
				seen[p] = struct{}{}
				paths = append(paths, p)
			}
		}
	}

	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}
		if block.Input == nil {
			continue
		}

		var input filePathInput
		if err := json.Unmarshal(block.Input, &input); err != nil {
			continue
		}

		switch block.Name {
		case "Read", "Edit", "Write":
			addPath(input.FilePath)
		case "NotebookEdit":
			addPath(input.NotebookPath)
		case "Glob", "Grep":
			addPath(input.Path)
		case "Bash":
			for _, m := range absPathRe.FindAllString(input.Command, -1) {
				addPath(m)
			}
		}
	}

	if len(paths) == 0 {
		return nil
	}
	return paths
}

// ResolveRepoRoot returns the git repository root for the given file path,
// or the path itself if it is not inside a git repository.
// Uses a process-scoped cache to avoid repeated git calls for paths in the same repo.
// Falls back to walking parent directories looking for .git/ if git is unavailable.
func ResolveRepoRoot(filePath string) string {
	dir := filepath.Dir(filePath)

	if cached, ok := repoRootCache.Load(dir); ok {
		return cached.(string)
	}

	// Try running git to find the repo root.
	root := resolveViaGit(dir)
	if root == "" {
		// git unavailable or path not in a repo — walk parent dirs for .git.
		root = resolveViaWalk(dir)
	}
	if root == "" {
		root = dir
	}

	repoRootCache.Store(dir, root)
	return root
}

// resolveViaGit runs git rev-parse --show-toplevel with a 2-second timeout.
// Returns empty string on failure.
func resolveViaGit(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return ""
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return absRoot
}

// resolveViaWalk walks parent directories looking for a .git entry.
// Returns the directory containing .git, or empty string if not found.
func resolveViaWalk(dir string) string {
	current := dir
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Lstat(gitPath); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding .git.
			break
		}
		current = parent
	}
	return ""
}

// ComputeProjectWeights aggregates file paths extracted from all entries in a
// session transcript into weighted project attribution. Each project's weight
// is the fraction of total tool calls that touched files in that repo.
// fallbackProject is used as the sole project (weight 1.0) if no file paths
// are extracted. Returns a non-nil slice sorted by Weight descending.
func ComputeProjectWeights(entries []TranscriptEntry, fallbackProject string) []ProjectWeight {
	toolCallsByRoot := make(map[string]int)
	total := 0

	for _, entry := range entries {
		paths := ExtractFilePaths(entry)
		for _, p := range paths {
			root := ResolveRepoRoot(p)
			toolCallsByRoot[root]++
			total++
		}
	}

	if total == 0 {
		return []ProjectWeight{
			{
				Project:   filepath.Base(fallbackProject),
				RepoRoot:  fallbackProject,
				Weight:    1.0,
				ToolCalls: 0,
			},
		}
	}

	weights := make([]ProjectWeight, 0, len(toolCallsByRoot))
	for root, count := range toolCallsByRoot {
		weights = append(weights, ProjectWeight{
			Project:   filepath.Base(root),
			RepoRoot:  root,
			Weight:    float64(count) / float64(total),
			ToolCalls: count,
		})
	}

	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Weight > weights[j].Weight
	})

	return weights
}
