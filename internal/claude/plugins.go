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

	// The file may be either a structured object {"version":…,"plugins":{…}}
	// or a plain map of plugin names to installation arrays. Try the
	// structured form first; fall back to the plain map.
	var installed InstalledPlugins
	if err := json.Unmarshal(data, &installed); err == nil && len(installed.Plugins) > 0 {
		var plugins []PluginEntry
		for name, installations := range installed.Plugins {
			version := ""
			if len(installations) > 0 {
				version = installations[0].Version
			}
			plugins = append(plugins, PluginEntry{
				Name:    name,
				Version: version,
			})
		}
		return plugins, nil
	}

	// Try parsing as a plain map: {"pluginName": [{...}], ...}
	var plainMap map[string][]PluginInstallation
	if err := json.Unmarshal(data, &plainMap); err == nil {
		var plugins []PluginEntry
		for name, installations := range plainMap {
			version := ""
			if len(installations) > 0 {
				version = installations[0].Version
			}
			plugins = append(plugins, PluginEntry{
				Name:    name,
				Version: version,
			})
		}
		return plugins, nil
	}

	// Try parsing as an array: [{...}, ...]
	var arr []PluginEntry
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}

	// If none of the formats worked, return empty rather than crashing.
	return nil, nil
}
