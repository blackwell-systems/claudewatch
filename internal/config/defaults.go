// Package config provides configuration loading and defaults for claudewatch.
package config

// DefaultScanPaths are the default directories to scan for projects.
var DefaultScanPaths = []string{"~/code"}

// DefaultClaudeHome is the default location of Claude Code's data directory.
const DefaultClaudeHome = "~/.claude"

// DefaultConfigDir is the default location for claudewatch configuration.
const DefaultConfigDir = "~/.config/claudewatch"

// DefaultDBName is the filename for the SQLite database.
const DefaultDBName = "claudewatch.db"

// DefaultConfigFile is the filename for the YAML config.
const DefaultConfigFile = "config.yaml"

// DefaultActiveThreshold is the minimum number of sessions for a project
// to be considered "active".
const DefaultActiveThreshold = 1

// DefaultWeights holds the default scoring weights for project readiness.
var DefaultWeights = Weights{
	ClaudeMDExists:    30,
	ClaudeMDQuality:   10,
	DotClaudeDir:      10,
	LocalSettings:     5,
	SessionHistory:    15,
	FacetsCoverage:    10,
	ActiveDevelopment: 10,
	HookAdoption:      5,
	PluginUsage:       5,
}

// DefaultFriction holds the default friction analysis thresholds.
var DefaultFriction = Friction{
	RecurringThreshold:  0.30,
	HighErrorMultiplier: 2.0,
}

// DefaultOutput holds the default output preferences.
var DefaultOutput = Output{
	Color: true,
	Width: 80,
}

// DefaultCustomMetrics provides the preset custom metric definitions.
var DefaultCustomMetrics = map[string]MetricDefinition{
	"session_quality": {
		Type:        "scale",
		Range:       [2]float64{1, 5},
		Direction:   "higher_is_better",
		Description: "Overall session quality rating",
	},
	"resume_callback": {
		Type:        "boolean",
		Direction:   "true_is_better",
		Description: "Did this tailored resume get a callback?",
	},
	"time_to_first_commit": {
		Type:        "duration",
		Direction:   "lower_is_better",
		Description: "Time from session start to first commit",
	},
	"scope_creep": {
		Type:        "boolean",
		Direction:   "false_is_better",
		Description: "Did Claude make unrequested changes?",
	},
}
