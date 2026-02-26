package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config is the top-level claudewatch configuration.
type Config struct {
	ScanPaths       []string                    `mapstructure:"scan_paths"`
	ClaudeHome      string                      `mapstructure:"claude_home"`
	ActiveThreshold int                         `mapstructure:"active_threshold"`
	Weights         Weights                     `mapstructure:"weights"`
	Friction        Friction                    `mapstructure:"friction"`
	Output          Output                      `mapstructure:"output"`
	CustomMetrics   map[string]MetricDefinition `mapstructure:"custom_metrics"`
}

// Weights defines the scoring weights for project readiness.
type Weights struct {
	ClaudeMDExists    float64 `mapstructure:"claude_md_exists"`
	ClaudeMDQuality   float64 `mapstructure:"claude_md_quality"`
	DotClaudeDir      float64 `mapstructure:"dot_claude_dir"`
	LocalSettings     float64 `mapstructure:"local_settings"`
	SessionHistory    float64 `mapstructure:"session_history"`
	FacetsCoverage    float64 `mapstructure:"facets_coverage"`
	ActiveDevelopment float64 `mapstructure:"active_development"`
	HookAdoption      float64 `mapstructure:"hook_adoption"`
	PluginUsage       float64 `mapstructure:"plugin_usage"`
}

// Friction defines thresholds for friction analysis.
type Friction struct {
	RecurringThreshold  float64 `mapstructure:"recurring_threshold"`
	HighErrorMultiplier float64 `mapstructure:"high_error_multiplier"`
}

// Output defines output preferences.
type Output struct {
	Color bool `mapstructure:"color"`
	Width int  `mapstructure:"width"`
}

// MetricDefinition describes a user-defined custom metric.
type MetricDefinition struct {
	Type        string     `mapstructure:"type"`
	Range       [2]float64 `mapstructure:"range"`
	Direction   string     `mapstructure:"direction"`
	Description string     `mapstructure:"description"`
}

// expandPath replaces a leading ~ with the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// Load reads configuration from the given path (or the default location)
// and returns a Config with all defaults applied.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults.
	v.SetDefault("scan_paths", DefaultScanPaths)
	v.SetDefault("claude_home", DefaultClaudeHome)
	v.SetDefault("active_threshold", DefaultActiveThreshold)
	v.SetDefault("weights.claude_md_exists", DefaultWeights.ClaudeMDExists)
	v.SetDefault("weights.claude_md_quality", DefaultWeights.ClaudeMDQuality)
	v.SetDefault("weights.dot_claude_dir", DefaultWeights.DotClaudeDir)
	v.SetDefault("weights.local_settings", DefaultWeights.LocalSettings)
	v.SetDefault("weights.session_history", DefaultWeights.SessionHistory)
	v.SetDefault("weights.facets_coverage", DefaultWeights.FacetsCoverage)
	v.SetDefault("weights.active_development", DefaultWeights.ActiveDevelopment)
	v.SetDefault("weights.hook_adoption", DefaultWeights.HookAdoption)
	v.SetDefault("weights.plugin_usage", DefaultWeights.PluginUsage)
	v.SetDefault("friction.recurring_threshold", DefaultFriction.RecurringThreshold)
	v.SetDefault("friction.high_error_multiplier", DefaultFriction.HighErrorMultiplier)
	v.SetDefault("output.color", DefaultOutput.Color)
	v.SetDefault("output.width", DefaultOutput.Width)

	if cfgFile != "" {
		v.SetConfigFile(expandPath(cfgFile))
	} else {
		configDir := expandPath(DefaultConfigDir)
		v.AddConfigPath(configDir)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Read config file if it exists; missing file is not an error.
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only return error for problems other than file not found.
			if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Apply custom metrics defaults if none configured.
	if len(cfg.CustomMetrics) == 0 {
		cfg.CustomMetrics = DefaultCustomMetrics
	}

	// Expand paths.
	cfg.ClaudeHome = expandPath(cfg.ClaudeHome)
	for i, p := range cfg.ScanPaths {
		cfg.ScanPaths[i] = expandPath(p)
	}

	return &cfg, nil
}

// DBPath returns the full path to the SQLite database.
func DBPath() string {
	return filepath.Join(expandPath(DefaultConfigDir), DefaultDBName)
}

// ConfigDir returns the expanded configuration directory.
func ConfigDir() string {
	return expandPath(DefaultConfigDir)
}
