package export

import (
	"encoding/json"
)

// JSONExporter outputs metrics in JSON format.
type JSONExporter struct{}

// Format returns "json".
func (j *JSONExporter) Format() string {
	return "json"
}

// Export renders the MetricSnapshot as pretty-printed JSON.
func (j *JSONExporter) Export(snapshot MetricSnapshot) ([]byte, error) {
	return json.MarshalIndent(snapshot, "", "  ")
}

// ExportMultiple renders multiple MetricSnapshots as a JSON array.
func (j *JSONExporter) ExportMultiple(snapshots []MetricSnapshot) ([]byte, error) {
	return json.MarshalIndent(snapshots, "", "  ")
}

// ExportDetailed renders per-session details as a JSON array.
func (j *JSONExporter) ExportDetailed(details []SessionDetail) ([]byte, error) {
	return json.MarshalIndent(details, "", "  ")
}
