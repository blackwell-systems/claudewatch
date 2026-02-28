package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ParseAllTodos reads all JSON files from ~/.claude/todos/ and returns
// parsed SessionTodos entries. Each file represents one session-agent pair.
func ParseAllTodos(claudeHome string) ([]SessionTodos, error) {
	dir := filepath.Join(claudeHome, "todos")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []SessionTodos
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var tasks []TodoTask
		if err := json.Unmarshal(data, &tasks); err != nil {
			continue
		}
		if len(tasks) == 0 {
			continue
		}

		sessionID, agentID := parseTodoFilename(entry.Name())
		results = append(results, SessionTodos{
			SessionID: sessionID,
			AgentID:   agentID,
			Tasks:     tasks,
		})
	}
	return results, nil
}

// parseTodoFilename extracts session and agent IDs from a filename like
// "{sessionID}-agent-{agentID}.json".
func parseTodoFilename(name string) (sessionID, agentID string) {
	name = strings.TrimSuffix(name, ".json")
	const sep = "-agent-"
	idx := strings.Index(name, sep)
	if idx < 0 {
		return name, ""
	}
	return name[:idx], name[idx+len(sep):]
}
