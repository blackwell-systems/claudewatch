// Package export provides metrics export capabilities for external observability platforms.
package export

import (
	"fmt"
)

// Exporter formats metrics for external platforms.
type Exporter interface {
	// Export renders a MetricSnapshot in the exporter's format.
	// Returns formatted output suitable for stdout or file write.
	Export(snapshot MetricSnapshot) ([]byte, error)

	// Format returns the format identifier (e.g., "prometheus", "json").
	Format() string
}

// MetricSnapshot is defined in metrics.go

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
