package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ParseAllSessionMeta walks ~/.claude/projects/<hash>/*.jsonl and returns a
// SessionMeta for every transcript file found. Results are loaded from a JSON
// cache when fresh; stale or missing caches are rebuilt from the JSONL and
// written back atomically. This makes all sessions visible — not just the 53%
// that have cached meta files written by Claude Code on clean exit.
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error) {
	projectsDir := filepath.Join(claudeHome, "projects")
	cacheDir := filepath.Join(claudeHome, "usage-data", "session-meta")

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []SessionMeta
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
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			sessionID := strings.TrimSuffix(f.Name(), ".jsonl")
			jsonlPath := filepath.Join(projDir, f.Name())
			cachePath := filepath.Join(cacheDir, sessionID+".json")
			meta, err := loadOrParseSession(jsonlPath, cachePath, cacheDir, sessionID)
			if err != nil || meta == nil {
				continue
			}
			results = append(results, *meta)
		}
	}
	return results, nil
}

// loadOrParseSession returns a SessionMeta from the cache if it is still fresh,
// otherwise parses the JSONL transcript and writes a new cache entry.
func loadOrParseSession(jsonlPath, cachePath, cacheDir, sessionID string) (*SessionMeta, error) {
	// Cache-hit condition: cache file exists AND jsonl mtime is NOT after cache mtime.
	jsonlInfo, jsonlErr := os.Stat(jsonlPath)
	cacheInfo, cacheErr := os.Stat(cachePath)
	if jsonlErr == nil && cacheErr == nil && !jsonlInfo.ModTime().After(cacheInfo.ModTime()) {
		// Try to load from cache.
		data, err := os.ReadFile(cachePath)
		if err == nil {
			var meta SessionMeta
			if err := json.Unmarshal(data, &meta); err == nil {
				return &meta, nil
			}
		}
		// Fall through to JSONL parse if cache read/unmarshal fails.
	}

	meta, err := ParseJSONLToSessionMeta(jsonlPath)
	if err == nil && meta != nil {
		_ = writeSessionMetaCache(cacheDir, sessionID, meta)
	}
	return meta, err
}

// ParseJSONLToSessionMeta performs a single-pass scan over a JSONL transcript
// file and derives a SessionMeta. It is the authoritative source for all
// fields derivable from the transcript; fields that only Claude Code can
// populate (Languages, LinesAdded, LinesRemoved, FilesModified, GitPushes,
// UserInterruptions) are left at their zero values.
// This function handles live (incomplete) sessions via line-atomic truncation.
func ParseJSONLToSessionMeta(jsonlPath string) (*SessionMeta, error) {
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return nil, err
	}

	// Line-atomic truncation: exclude any unterminated trailing line.
	if lastNL := bytes.LastIndexByte(data, '\n'); lastNL >= 0 {
		data = data[:lastNL+1]
	} else {
		// No complete lines — return a minimal but non-nil struct.
		meta := &SessionMeta{
			ToolCounts:          make(map[string]int),
			ToolErrorCategories: make(map[string]int),
			ModelUsage:          make(map[string]ModelStats),
		}
		meta.SessionID = strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
		meta.ProjectPath = filepath.Base(filepath.Dir(jsonlPath))
		return meta, nil
	}

	var meta SessionMeta
	meta.ToolCounts = make(map[string]int)
	meta.ToolErrorCategories = make(map[string]int)
	meta.ModelUsage = make(map[string]ModelStats)

	var startTimeSet bool
	var firstEntryTime, lastEntryTime time.Time
	var lastAssistantTime time.Time
	var firstPromptSet bool

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// 10MB buffer, same as existing JSONL parsers in this package.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		// Always: populate session ID from first non-empty value.
		if meta.SessionID == "" && entry.SessionID != "" {
			meta.SessionID = entry.SessionID
		}
		// Always: populate project path from first non-empty cwd.
		if meta.ProjectPath == "" && entry.Cwd != "" {
			meta.ProjectPath = entry.Cwd
		}
		// Always: populate start time from first timestamped entry.
		if !startTimeSet && entry.Timestamp != "" {
			t := ParseTimestamp(entry.Timestamp)
			if !t.IsZero() {
				meta.StartTime = t.Format(time.RFC3339)
				firstEntryTime = t
				startTimeSet = true
				_ = firstEntryTime // used below for DurationMinutes
			}
		}
		// Always: track last entry time and hour-of-day.
		if entry.Timestamp != "" {
			lastEntryTime = ParseTimestamp(entry.Timestamp)
			if !lastEntryTime.IsZero() {
				meta.MessageHours = append(meta.MessageHours, lastEntryTime.Hour())
			}
		}

		switch entry.Type {
		case "assistant":
			meta.AssistantMessageCount++
			if entry.Timestamp != "" {
				lastAssistantTime = ParseTimestamp(entry.Timestamp)
			}
			if entry.Message != nil {
				var msg assistantMsgWithContent
				if err := json.Unmarshal(entry.Message, &msg); err == nil {
					meta.InputTokens += msg.Usage.InputTokens
					meta.OutputTokens += msg.Usage.OutputTokens

					// Track per-model token usage.
					if msg.Model != "" {
						stats := meta.ModelUsage[msg.Model]
						stats.InputTokens += msg.Usage.InputTokens
						stats.OutputTokens += msg.Usage.OutputTokens
						meta.ModelUsage[msg.Model] = stats
					}

					for _, block := range msg.Content {
						if block.Type != "tool_use" {
							continue
						}
						meta.ToolCounts[block.Name]++
						switch {
						case block.Name == "Task":
							meta.UsesTaskAgent = true
						case strings.HasPrefix(block.Name, "mcp__"):
							meta.UsesMCP = true
						case block.Name == "WebSearch":
							meta.UsesWebSearch = true
						case block.Name == "WebFetch":
							meta.UsesWebFetch = true
						}
					}
				}
			}

		case "user":
			meta.UserMessageCount++
			if entry.Timestamp != "" {
				meta.UserMessageTimestamps = append(meta.UserMessageTimestamps, entry.Timestamp)
			}
			if !lastAssistantTime.IsZero() && entry.Timestamp != "" {
				t := ParseTimestamp(entry.Timestamp)
				if !t.IsZero() {
					meta.UserResponseTimes = append(meta.UserResponseTimes, t.Sub(lastAssistantTime).Seconds())
				}
				lastAssistantTime = time.Time{}
			}
			if entry.Message != nil {
				var msg UserMessage
				if err := json.Unmarshal(entry.Message, &msg); err == nil {
					if !firstPromptSet {
						for _, block := range msg.Content {
							if block.Type == "text" {
								text := block.Text
								if len(text) > 500 {
									text = text[:500]
								}
								meta.FirstPrompt = text
								firstPromptSet = true
								break
							}
						}
					}
					for _, block := range msg.Content {
						if block.Type == "tool_result" && block.IsError {
							meta.ToolErrors++
						}
					}
				}
			}
		}
	}

	// Post-scan: fill fallback identifiers.
	if meta.SessionID == "" {
		meta.SessionID = strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
	}
	if meta.ProjectPath == "" {
		meta.ProjectPath = filepath.Base(filepath.Dir(jsonlPath))
	}

	// Compute duration from first to last timestamped entry.
	if startTimeSet && !lastEntryTime.IsZero() {
		startT := ParseTimestamp(meta.StartTime)
		meta.DurationMinutes = int(lastEntryTime.Sub(startT).Minutes())
	}

	// Count git commits since session start (non-fatal).
	if meta.ProjectPath != "" && meta.StartTime != "" {
		meta.GitCommits = countGitCommitsSince(meta.ProjectPath, meta.StartTime)
	}

	// Filter out bootstrap/ephemeral sessions: these are created momentarily
	// when resuming a session to run hooks and load context, then immediately
	// hand off to the real resumed session. They pollute metrics because they
	// have zero actual work but count as sessions. Heuristic: no conversation
	// turns (assistant or user messages) indicates a bootstrap session.
	if meta.AssistantMessageCount == 0 && meta.UserMessageCount == 0 {
		return nil, nil
	}

	return &meta, nil
}

// assistantMsgWithContent is an internal type that extends assistantMsgUsage
// with the content blocks needed to extract tool-use information.
type assistantMsgWithContent struct {
	Model   string         `json:"model"`
	Content []ContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// writeSessionMetaCache atomically writes meta as a JSON file into cacheDir
// using a temp-file + rename pattern to avoid partial writes.
func writeSessionMetaCache(cacheDir, sessionID string, meta *SessionMeta) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(cacheDir, sessionID+".json.tmp")
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath.Join(cacheDir, sessionID+".json"))
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
