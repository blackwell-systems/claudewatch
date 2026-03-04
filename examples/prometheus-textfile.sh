#!/bin/bash
# Write claudewatch metrics to Prometheus node exporter textfile collector
#
# This script exports metrics to a file that Prometheus node exporter can scrape.
# The node exporter must be configured with --collector.textfile.directory pointing
# to the directory specified by TEXTFILE_DIR.
#
# Usage:
#   ./prometheus-textfile.sh [PROJECT]
#
# Environment variables:
#   TEXTFILE_DIR - Directory for textfile collector (default: /var/lib/node_exporter/textfile_collector)
#   DAYS         - Time window in days (default: 30)
#
# Examples:
#   # Write metrics for all projects
#   ./prometheus-textfile.sh
#
#   # Write metrics for specific project
#   PROJECT=myapp ./prometheus-textfile.sh
#
#   # Write to custom directory
#   TEXTFILE_DIR=/tmp/metrics ./prometheus-textfile.sh
#
#   # Write last 7 days only
#   DAYS=7 ./prometheus-textfile.sh
#
# Setup:
#   1. Create textfile collector directory:
#      sudo mkdir -p /var/lib/node_exporter/textfile_collector
#      sudo chown prometheus:prometheus /var/lib/node_exporter/textfile_collector
#
#   2. Configure node exporter:
#      node_exporter --collector.textfile.directory=/var/lib/node_exporter/textfile_collector
#
#   3. Add to crontab for regular updates:
#      0 * * * * /usr/local/bin/prometheus-textfile.sh

set -euo pipefail

# Configuration
TEXTFILE_DIR="${TEXTFILE_DIR:-/var/lib/node_exporter/textfile_collector}"
PROJECT="${1:-${PROJECT:-}}"
DAYS="${DAYS:-30}"

# Determine output filename
if [ -n "$PROJECT" ]; then
  OUTPUT_FILE="$TEXTFILE_DIR/claudewatch-${PROJECT}.prom"
  echo "Writing metrics for project: $PROJECT (last $DAYS days)"
else
  OUTPUT_FILE="$TEXTFILE_DIR/claudewatch.prom"
  echo "Writing metrics for all projects (last $DAYS days)"
fi

# Ensure directory exists and is writable
if [ ! -d "$TEXTFILE_DIR" ]; then
  echo "Error: Directory $TEXTFILE_DIR does not exist" >&2
  echo "Create it with: sudo mkdir -p $TEXTFILE_DIR" >&2
  exit 1
fi

if [ ! -w "$TEXTFILE_DIR" ]; then
  echo "Error: Directory $TEXTFILE_DIR is not writable" >&2
  echo "Fix permissions with: sudo chown $USER:$USER $TEXTFILE_DIR" >&2
  exit 1
fi

# Build claudewatch export command
EXPORT_CMD="claudewatch export --format prometheus --days $DAYS"
if [ -n "$PROJECT" ]; then
  EXPORT_CMD="$EXPORT_CMD --project $PROJECT"
fi

# Write to temp file first (atomic update)
TMP_FILE=$(mktemp)
trap "rm -f $TMP_FILE" EXIT

$EXPORT_CMD > "$TMP_FILE"

if [ $? -ne 0 ]; then
  echo "✗ Failed to export metrics" >&2
  exit 1
fi

# Atomic move to final location
mv "$TMP_FILE" "$OUTPUT_FILE"

if [ $? -eq 0 ]; then
  echo "✓ Metrics written to $OUTPUT_FILE"
  echo "  File size: $(du -h "$OUTPUT_FILE" | cut -f1)"
  echo "  Metrics: $(grep -c "^claudewatch_" "$OUTPUT_FILE" || echo 0)"
else
  echo "✗ Failed to write metrics file" >&2
  exit 1
fi
