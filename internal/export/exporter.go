// Package export provides metrics export capabilities for external observability platforms.
package export

import (
	"fmt"
	"time"
)

// Exporter formats metrics for external platforms.
type Exporter interface {
	// Export renders a MetricSnapshot in the exporter's format.
	// Returns formatted output suitable for stdout or file write.
	Export(snapshot MetricSnapshot) ([]byte, error)

	// Format returns the format identifier (e.g., "prometheus", "json").
	Format() string
}

// MetricSnapshot contains safe, aggregated metrics for export.
// No sensitive data (transcript content, file paths, credentials).
type MetricSnapshot struct {
	Timestamp time.Time

	// Project identity (hash or name, never absolute paths)
	ProjectName string
	ProjectHash string

	// Session metrics
	SessionCount     int
	TotalDurationMin float64
	AvgDurationMin   float64
	ActiveMinutes    float64

	// Friction metrics
	FrictionRate   float64        // sessions with friction / total sessions
	FrictionByType map[string]int // friction event counts by type
	AvgToolErrors  float64

	// Productivity metrics
	TotalCommits         int
	AvgCommitsPerSession float64
	CommitAttemptRatio   float64 // commits / (Edit+Write tool uses)
	ZeroCommitRate       float64

	// Cost metrics (USD)
	TotalCostUSD      float64
	AvgCostPerSession float64
	CostPerCommit     float64

	// Model usage (percentages, not token counts)
	ModelUsagePct map[string]float64 // model name → % of sessions

	// Agent metrics
	AgentSuccessRate float64
	AgentUsageRate   float64 // sessions with agents / total

	// Context pressure (aggregated status)
	AvgContextPressure float64 // 0.0-1.0
}

// Exporter registry
var exporters = map[string]Exporter{
	"prometheus": &PrometheusExporter{},
}

// GetExporter returns the exporter for the specified format.
func GetExporter(format string) (Exporter, error) {
	e, ok := exporters[format]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return e, nil
}
