package export

import (
	"fmt"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/config"
)

// CollectMetrics gathers safe, aggregated metrics for export.
// This is a stub implementation - Agent B will provide the full implementation.
//
// projectFilter: empty string = all projects, or specific project name
// days: time window (0 = all time)
func CollectMetrics(cfg *config.Config, projectFilter string, days int) (MetricSnapshot, error) {
	// TODO(Agent B): Implement actual metric collection from analyzer package
	// This stub allows Agent A's code to compile and be tested independently.
	return MetricSnapshot{
		Timestamp:   time.Now(),
		ProjectName: projectFilter,
	}, fmt.Errorf("CollectMetrics not yet implemented - waiting for Agent B")
}
