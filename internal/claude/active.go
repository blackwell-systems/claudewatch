package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ActiveSessionInfo wraps a parsed live session with its source path.
type ActiveSessionInfo struct {
	SessionMeta
	Path   string
	IsLive bool
}

// assistantMsgUsage is an internal type used to extract token usage from
// assistant message entries in a JSONL transcript.
type assistantMsgUsage struct {
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// FindActiveSessionPath scans ~/.claude/projects/**/*.jsonl for a file
// currently open by a Claude Code process.
// Returns ("", nil) if no active session is found.
// Returns ("", error) only on unexpected I/O failure.
func FindActiveSessionPath(claudeHome string) (string, error) {
	projectsDir := filepath.Join(claudeHome, "projects")

	// Enumerate all .jsonl files under projects/<hash>/<session>.jsonl.
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var jsonlFiles []string
	var newestPath string
	var newestMtime time.Time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(projectsDir, entry.Name())
		files, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			fullPath := filepath.Join(dirPath, f.Name())
			jsonlFiles = append(jsonlFiles, fullPath)

			// Track the most recently modified file for fallback.
			fi, err := f.Info()
			if err != nil {
				continue
			}
			if fi.ModTime().After(newestMtime) {
				newestMtime = fi.ModTime()
				newestPath = fullPath
			}
		}
	}

	if len(jsonlFiles) == 0 {
		return "", nil
	}

	// Primary detection method: use lsof to find open files.
	if found := findOpenFileWithLsof(jsonlFiles); found != "" {
		return found, nil
	}

	// Fallback: check if the most recently modified file is within 5 minutes.
	if newestPath != "" && time.Since(newestMtime) < 5*time.Minute {
		return newestPath, nil
	}

	return "", nil
}

// findOpenFileWithLsof runs lsof -F n with a 3-second timeout and returns
// the first path from jsonlFiles that appears in the lsof output.
// Returns "" if lsof is unavailable, times out, or no match is found.
func findOpenFileWithLsof(jsonlFiles []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lsof", "-F", "n")
	out, err := cmd.Output()
	if err != nil {
		// lsof failure is non-fatal; fall through to mtime heuristic.
		return ""
	}

	// Build a set of the JSONL paths for O(1) lookup.
	pathSet := make(map[string]bool, len(jsonlFiles))
	for _, p := range jsonlFiles {
		pathSet[p] = true
	}

	// Parse lsof output: lines starting with 'n' carry a filename.
	// Example: n/Users/dayna/.claude/projects/abc/session.jsonl
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "n") {
			continue
		}
		filePath := line[1:] // strip leading 'n'
		if pathSet[filePath] {
			return filePath
		}
	}

	return ""
}

// ParseActiveSession reads the JSONL file at path as a partial transcript
// and reconstructs a *SessionMeta. Returns a non-nil *SessionMeta on
// success (even if partially populated). Returns (nil, error) only on
// file read failure.
func ParseActiveSession(path string) (*SessionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Line-atomic truncation: find the last newline byte and truncate there,
	// so that any partial (unterminated) trailing line is excluded.
	if lastNL := bytes.LastIndexByte(data, '\n'); lastNL >= 0 {
		data = data[:lastNL+1]
	} else {
		// No complete lines yet — return an empty but non-nil struct.
		return &SessionMeta{}, nil
	}

	var meta SessionMeta
	var startTimeSet bool

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase buffer for long JSONL lines (up to 10MB), same as ParseSingleTranscript.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines silently.
			continue
		}

		// Populate SessionID from the first non-empty entry value.
		if meta.SessionID == "" && entry.SessionID != "" {
			meta.SessionID = entry.SessionID
		}

		// Populate ProjectPath from the cwd field on the SessionStart progress entry.
		if meta.ProjectPath == "" && entry.Cwd != "" {
			meta.ProjectPath = entry.Cwd
		}

		// Populate StartTime from the first entry with a non-zero timestamp.
		if !startTimeSet && entry.Timestamp != "" {
			t := ParseTimestamp(entry.Timestamp)
			if !t.IsZero() {
				meta.StartTime = t.Format(time.RFC3339)
				startTimeSet = true
			}
		}

		switch entry.Type {
		case "user":
			meta.UserMessageCount++
		case "assistant":
			meta.AssistantMessageCount++

			// Attempt to extract token usage from the assistant message.
			if entry.Message != nil {
				var msg assistantMsgUsage
				if err := json.Unmarshal(entry.Message, &msg); err == nil {
					meta.InputTokens += msg.Usage.InputTokens
					meta.OutputTokens += msg.Usage.OutputTokens
				}
			}
		}
	}

	// If no entry carried a sessionId, derive it from the filename.
	if meta.SessionID == "" {
		meta.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}

	// ProjectPath is populated from the cwd field in the JSONL (real filesystem path).
	// Fall back to the hash directory name only when cwd was absent from all entries.
	if meta.ProjectPath == "" {
		meta.ProjectPath = filepath.Base(filepath.Dir(path))
	}

	return &meta, nil
}
