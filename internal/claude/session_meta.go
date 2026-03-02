package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ParseAllSessionMeta reads all JSON files from ~/.claude/usage-data/session-meta/
// and returns parsed SessionMeta entries. For any session whose JSONL transcript
// is newer than the cached meta JSON (i.e. a still-active session), the message
// counts and token totals are refreshed from the JSONL so callers always see
// up-to-date progress. Fields written exclusively by Claude Code (git commits,
// languages, lines changed, etc.) are preserved from the JSON.
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error) {
	dir := filepath.Join(claudeHome, "usage-data", "session-meta")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Index all JSONL session files by session ID so we can detect staleness.
	jsonlIndex := buildSessionJSONLIndex(claudeHome)

	var results []SessionMeta
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		metaPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		// If the JSONL is newer than the meta JSON, the session is still active
		// and the cached counts are stale. Re-parse the JSONL and overlay only
		// the fields we can derive from the transcript.
		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		if jsonlPath, ok := jsonlIndex[sessionID]; ok {
			metaInfo, metaErr := entry.Info()
			jsonlInfo, jsonlErr := os.Stat(jsonlPath)
			if metaErr == nil && jsonlErr == nil && jsonlInfo.ModTime().After(metaInfo.ModTime()) {
				if live, err := ParseActiveSession(jsonlPath); err == nil && live != nil {
					meta.UserMessageCount = live.UserMessageCount
					meta.AssistantMessageCount = live.AssistantMessageCount
					meta.InputTokens = live.InputTokens
					meta.OutputTokens = live.OutputTokens
					meta.UserMessageTimestamps = live.UserMessageTimestamps
					// Recompute duration from first message to now.
					if meta.StartTime != "" {
						if t := ParseTimestamp(meta.StartTime); !t.IsZero() {
							meta.DurationMinutes = int(time.Since(t).Minutes())
						}
					}
					// Count commits made in the project repo since session start.
					// Non-fatal: returns 0 if path is empty, not a git repo, or git fails.
					if meta.ProjectPath != "" && meta.StartTime != "" {
						meta.GitCommits = countGitCommitsSince(meta.ProjectPath, meta.StartTime)
					}
				}
			}
		}

		results = append(results, meta)
	}
	return results, nil
}

// buildSessionJSONLIndex walks ~/.claude/projects/ and returns a map of
// sessionID → absolute JSONL path. Subagent directories (nested dirs) are skipped.
func buildSessionJSONLIndex(claudeHome string) map[string]string {
	index := make(map[string]string)
	projectsDir := filepath.Join(claudeHome, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return index
	}
	for _, proj := range entries {
		if !proj.IsDir() {
			continue
		}
		projDir := filepath.Join(projectsDir, proj.Name())
		files, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			// Skip subdirectories (subagent session dirs live under <sessionID>/subagents/).
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			sessionID := strings.TrimSuffix(f.Name(), ".jsonl")
			index[sessionID] = filepath.Join(projDir, f.Name())
		}
	}
	return index
}

// ParseSessionMeta reads a single session meta file.
func ParseSessionMeta(path string) (*SessionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// countGitCommitsSince counts commits made after startTime in git repos
// at or under projectPath. If projectPath is itself a git repo, only that
// repo is counted. If it is not a git repo (e.g. a workspace directory like
// ~/code/), all immediate subdirectories that are git repos are scanned and
// their commit counts summed — covering the common pattern of launching from
// a parent directory and working across multiple repos in one session.
// Returns 0 on any error; missing git, non-repo paths, and empty inputs all
// silently return 0.
func countGitCommitsSince(projectPath, startTime string) int {
	if projectPath == "" || startTime == "" {
		return 0
	}
	if isGitRepo(projectPath) {
		return gitCommitCount(projectPath, startTime)
	}
	// Not a git repo — scan one level of subdirectories.
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return 0
	}
	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(projectPath, e.Name())
		if isGitRepo(sub) {
			total += gitCommitCount(sub, startTime)
		}
	}
	return total
}

// isGitRepo reports whether dir is the root of a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// gitCommitCount returns the number of commits in the repo at dir made after
// startTime. Returns 0 on any error.
func gitCommitCount(dir, startTime string) int {
	cmd := exec.Command("git", "-C", dir, "log", "--oneline", "--after="+startTime)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return 0
	}
	return bytes.Count(out, []byte("\n")) + 1
}

// parseJSONDir reads all .json files from a directory and unmarshals them
// into a slice of the given type. Skips files that fail to parse.
func parseJSONDir[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []T
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}
