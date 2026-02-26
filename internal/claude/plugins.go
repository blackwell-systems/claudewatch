package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ParsePlugins reads ~/.claude/plugins/installed_plugins.json and returns
// the list of installed plugins. Returns nil if the file does not exist.
func ParsePlugins(claudeHome string) ([]PluginEntry, error) {
	path := filepath.Join(claudeHome, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []PluginEntry
	if err := json.Unmarshal(data, &plugins); err != nil {
		return nil, err
	}
	return plugins, nil
}
