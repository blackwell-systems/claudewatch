package app

import (
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/export"
	"github.com/spf13/cobra"
)

var (
	exportFormat        string
	exportProject       string
	exportDays          int
	exportOutput        string
	exportPerProject    bool
	exportPerDay        bool
	exportPerModel      bool
	exportSAWComparison bool
	exportDetailed      bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export metrics to external observability platforms",
	Long: `Export aggregated metrics in formats consumable by external observability platforms.

Exported metrics include session counts, friction rates, productivity metrics,
cost data, and agent performance. No sensitive data (transcript content, file
paths, or credentials) is included.

Supported formats:
  - json: Pretty-printed JSON (suitable for jq, analysis tools)
  - prometheus: Prometheus text format
  - csv: CSV with headers (suitable for spreadsheets)

Output to stdout by default, or specify --output to write to a file.

Granular reporting options:
  --per-project      One row/object per project instead of aggregate
  --per-day          Daily time series over the window
  --per-model        Split metrics by model (Sonnet vs Opus separately)
  --saw-comparison   Compare SAW sessions vs non-SAW sessions side-by-side
  --detailed         Output session-level rows (all sessions, not aggregated)

Examples:
  claudewatch export --per-project              # One row per project
  claudewatch export --per-day --days 30        # Daily time series
  claudewatch export --saw-comparison           # SAW vs non-SAW comparison
  claudewatch export --detailed --format csv    # Session-level CSV export
  claudewatch export --per-model --days 7       # Last 7 days by model`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Export format (json|prometheus|csv)")
	exportCmd.Flags().StringVar(&exportProject, "project", "", "Filter to specific project (empty = all)")
	exportCmd.Flags().IntVar(&exportDays, "days", 30, "Time window in days (0 = all time)")
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file path (default: stdout)")
	exportCmd.Flags().BoolVar(&exportPerProject, "per-project", false, "Output one row/object per project")
	exportCmd.Flags().BoolVar(&exportPerDay, "per-day", false, "Output daily time series")
	exportCmd.Flags().BoolVar(&exportPerModel, "per-model", false, "Split metrics by model")
	exportCmd.Flags().BoolVar(&exportSAWComparison, "saw-comparison", false, "Compare SAW vs non-SAW sessions")
	exportCmd.Flags().BoolVar(&exportDetailed, "detailed", false, "Output session-level rows (not aggregated)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate flag combinations
	if err := validateExportFlags(); err != nil {
		return err
	}

	// Get the appropriate exporter
	exporter, err := export.GetExporter(exportFormat)
	if err != nil {
		return err
	}

	var output []byte

	// Collect metrics based on flags
	if exportDetailed {
		// Detailed mode: per-session export
		details, err := export.CollectDetailedMetrics(cfg, exportProject, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect detailed metrics: %w", err)
		}
		output, err = exporter.ExportDetailed(details)
		if err != nil {
			return fmt.Errorf("failed to export detailed metrics: %w", err)
		}
	} else if exportSAWComparison {
		// SAW comparison mode
		saw, nonSAW, err := export.CollectSAWComparison(cfg, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect SAW comparison: %w", err)
		}
		output, err = exporter.ExportMultiple([]export.MetricSnapshot{saw, nonSAW})
		if err != nil {
			return fmt.Errorf("failed to export SAW comparison: %w", err)
		}
	} else if exportPerProject {
		// Per-project mode
		snapshots, err := export.CollectMetricsPerProject(cfg, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect per-project metrics: %w", err)
		}
		output, err = exporter.ExportMultiple(snapshots)
		if err != nil {
			return fmt.Errorf("failed to export per-project metrics: %w", err)
		}
	} else if exportPerDay {
		// Per-day mode
		snapshots, err := export.CollectMetricsPerDay(cfg, exportProject, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect per-day metrics: %w", err)
		}
		output, err = exporter.ExportMultiple(snapshots)
		if err != nil {
			return fmt.Errorf("failed to export per-day metrics: %w", err)
		}
	} else if exportPerModel {
		// Per-model mode
		modelMetrics, err := export.CollectMetricsPerModel(cfg, exportProject, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect per-model metrics: %w", err)
		}
		// Convert map to slice for export
		var snapshots []export.MetricSnapshot
		for modelName, snapshot := range modelMetrics {
			// Use model name in the project field for identification
			snapshot.ProjectName = modelName
			snapshots = append(snapshots, snapshot)
		}
		output, err = exporter.ExportMultiple(snapshots)
		if err != nil {
			return fmt.Errorf("failed to export per-model metrics: %w", err)
		}
	} else {
		// Default: single aggregate snapshot
		snapshot, err := export.CollectMetrics(cfg, exportProject, exportDays)
		if err != nil {
			return fmt.Errorf("failed to collect metrics: %w", err)
		}
		output, err = exporter.Export(snapshot)
		if err != nil {
			return fmt.Errorf("failed to export metrics: %w", err)
		}
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

func validateExportFlags() error {
	// Count active flags
	activeFlags := 0
	if exportPerProject {
		activeFlags++
	}
	if exportPerDay {
		activeFlags++
	}
	if exportPerModel {
		activeFlags++
	}
	if exportSAWComparison {
		activeFlags++
	}
	if exportDetailed {
		activeFlags++
	}

	// --detailed is mutually exclusive with all other flags
	if exportDetailed && activeFlags > 1 {
		return fmt.Errorf("--detailed cannot be combined with other grouping flags")
	}

	// --saw-comparison is standalone
	if exportSAWComparison && activeFlags > 1 {
		return fmt.Errorf("--saw-comparison cannot be combined with other grouping flags")
	}

	// --per-project and --per-day can be combined, but not implemented yet
	if exportPerProject && exportPerDay {
		return fmt.Errorf("combining --per-project and --per-day is not yet implemented")
	}

	// --per-model can't be combined with --per-project or --per-day yet
	if exportPerModel && (exportPerProject || exportPerDay) {
		return fmt.Errorf("combining --per-model with other flags is not yet implemented")
	}

	return nil
}
