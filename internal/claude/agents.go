package claude

import (
	"os"
	"path/filepath"
)

// ParseAgentTasks scans /tmp/claude-*/tasks/*.output for agent task output files
// and returns parsed AgentTask entries. These files are ephemeral and may not
// exist after a system reboot.
func ParseAgentTasks() ([]AgentTask, error) {
	pattern := filepath.Join(os.TempDir(), "claude-*", "tasks", "*.output")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var tasks []AgentTask
	for _, match := range matches {
		parsed, err := parseAgentOutputFile(match)
		if err != nil {
			// Skip files that fail to parse.
			continue
		}
		tasks = append(tasks, parsed...)
	}
	return tasks, nil
}

// parseAgentOutputFile parses a single agent output file. These are JSONL files
// containing agent transcript entries. This is a stub that returns the file path
// as a minimal placeholder; the full parsing logic will extract timestamps,
// tool uses, token counts, and completion status from the JSONL entries.
func parseAgentOutputFile(path string) ([]AgentTask, error) {
	// Verify the file is readable.
	_, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// TODO: Implement full JSONL parsing of agent output files.
	// Agent output files contain transcript entries with varying schemas.
	// Full implementation will extract: agent_id, agent_type, description,
	// session_id, status, duration, tokens, tool_uses, background flag.
	return nil, nil
}
