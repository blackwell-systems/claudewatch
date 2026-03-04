#!/bin/bash
# Examples demonstrating granular export options in claudewatch

set -e

echo "=== Claudewatch Export Examples ==="
echo ""

# 1. Basic aggregate export (default)
echo "1. Basic aggregate export (all sessions, last 30 days):"
echo "   claudewatch export --format json --days 30"
echo ""

# 2. Per-project breakdown
echo "2. Per-project breakdown:"
echo "   claudewatch export --per-project --format csv --output metrics-by-project.csv"
echo "   Generates one row per project showing aggregate metrics for each project"
echo ""

# 3. Daily time series
echo "3. Daily time series (last 14 days):"
echo "   claudewatch export --per-day --days 14 --format json"
echo "   Shows daily trends in friction, cost, and productivity"
echo ""

# 4. Per-model comparison
echo "4. Per-model comparison:"
echo "   claudewatch export --per-model --days 7 --format csv"
echo "   Compare metrics between Sonnet, Opus, and other models"
echo ""

# 5. SAW comparison
echo "5. Scout-and-Wave vs non-SAW comparison:"
echo "   claudewatch export --saw-comparison --format json"
echo "   Compare sessions that used Scout-and-Wave vs regular sessions"
echo ""

# 6. Detailed session export
echo "6. Detailed session-level export:"
echo "   claudewatch export --detailed --format csv --output sessions.csv"
echo "   Export every session as a separate row with all metrics"
echo ""

# 7. Filter by project
echo "7. Filter to specific project:"
echo "   claudewatch export --project claudewatch --days 7 --format json"
echo "   Only show metrics for the claudewatch project"
echo ""

# 8. Prometheus format for monitoring
echo "8. Prometheus format (per-project):"
echo "   claudewatch export --per-project --format prometheus > /var/lib/prometheus/textfile/claudewatch.prom"
echo "   Export in Prometheus text format for monitoring"
echo ""

# 9. Daily trends for a specific project
echo "9. Daily trends for a specific project:"
echo "   claudewatch export --per-day --project myproject --days 30 --format csv"
echo "   Daily time series for a single project"
echo ""

# 10. All-time statistics
echo "10. All-time statistics:"
echo "    claudewatch export --days 0 --format json"
echo "    Export metrics across all sessions (no time filter)"
echo ""

echo "=== Advanced Use Cases ==="
echo ""

# Use jq to analyze per-project metrics
echo "11. Find most expensive projects:"
echo '    claudewatch export --per-project --format json | jq "sort_by(.TotalCostUSD) | reverse | .[0:5]"'
echo ""

# Generate daily report for tracking progress
echo "12. Generate daily cost report:"
echo '    claudewatch export --per-day --days 30 --format json | jq ".[] | {date: .Timestamp, cost: .TotalCostUSD, commits: .TotalCommits}"'
echo ""

# Compare SAW effectiveness
echo "13. Compare SAW cost efficiency:"
echo '    claudewatch export --saw-comparison --format json | jq ".[] | {type: .ProjectName, cost_per_commit: .CostPerCommit}"'
echo ""

# Export to spreadsheet for analysis
echo "14. Export to Excel-compatible CSV:"
echo "    claudewatch export --detailed --format csv --output detailed-sessions.csv"
echo "    Open detailed-sessions.csv in Excel/Google Sheets for pivot table analysis"
echo ""

# Prometheus monitoring setup
echo "15. Set up Prometheus monitoring (cron job):"
echo '    # Add to crontab:'
echo '    */5 * * * * claudewatch export --format prometheus > /var/lib/prometheus/textfile/claudewatch.prom'
echo ""

echo "=== Flag Combinations ==="
echo ""
echo "Valid combinations:"
echo "  - None (default aggregate)"
echo "  - --per-project alone"
echo "  - --per-day alone"
echo "  - --per-model alone"
echo "  - --saw-comparison alone"
echo "  - --detailed alone"
echo ""
echo "Mutually exclusive:"
echo "  - --detailed cannot be combined with other flags"
echo "  - --saw-comparison cannot be combined with other flags"
echo "  - --per-project and --per-day combination not yet implemented"
echo ""

echo "=== Output Formats ==="
echo ""
echo "json:       Pretty-printed JSON, suitable for jq processing"
echo "csv:        CSV with headers, suitable for spreadsheets"
echo "prometheus: Prometheus text format, suitable for monitoring systems"
echo ""
