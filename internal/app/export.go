package app

import (
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/export"
	"github.com/spf13/cobra"
)

var (
	exportFormat  string
	exportProject string
	exportDays    int
	exportOutput  string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export metrics to external observability platforms",
	Long: `Export aggregated metrics in formats consumable by external observability platforms.

Exported metrics include session counts, friction rates, productivity metrics,
cost data, and agent performance. No sensitive data (transcript content, file
paths, or credentials) is included.

Supported formats:
  - prometheus: Prometheus text format (default)

Output to stdout by default, or specify --output to write to a file.`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "prometheus", "Export format (prometheus)")
	exportCmd.Flags().StringVar(&exportProject, "project", "", "Filter to specific project (empty = all)")
	exportCmd.Flags().IntVar(&exportDays, "days", 30, "Time window in days (0 = all time)")
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file path (default: stdout)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get the appropriate exporter
	exporter, err := export.GetExporter(exportFormat)
	if err != nil {
		return err
	}

	// Collect metrics (Agent B will implement CollectMetrics)
	// For now, we'll create a placeholder that Agent B will replace
	snapshot, err := export.CollectMetrics(cfg, exportProject, exportDays)
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	// Export to the specified format
	output, err := exporter.Export(snapshot)
	if err != nil {
		return fmt.Errorf("failed to export metrics: %w", err)
	}

	// Write to stdout or file
	if exportOutput == "" {
		// Write to stdout
		fmt.Print(string(output))
	} else {
		// Write to file
		if err := os.WriteFile(exportOutput, output, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Metrics exported to %s\n", exportOutput)
	}

	return nil
}
