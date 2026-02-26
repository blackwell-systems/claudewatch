package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ParseSettings reads ~/.claude/settings.json and returns the parsed settings.
func ParseSettings(claudeHome string) (*GlobalSettings, error) {
	path := filepath.Join(claudeHome, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var settings GlobalSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}
