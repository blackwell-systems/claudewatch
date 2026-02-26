package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ParseStatsCache reads ~/.claude/stats-cache.json and returns the parsed stats.
func ParseStatsCache(claudeHome string) (*StatsCache, error) {
	path := filepath.Join(claudeHome, "stats-cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var stats StatsCache
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
