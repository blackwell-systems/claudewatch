#!/bin/bash
# Push claudewatch metrics to Prometheus Pushgateway
#
# Usage:
#   ./prometheus-pushgateway.sh [PROJECT]
#
# Environment variables:
#   PUSHGATEWAY_URL - URL of the Pushgateway (default: http://localhost:9091)
#   DAYS            - Time window in days (default: 30)
#
# Examples:
#   # Push all projects
#   ./prometheus-pushgateway.sh
#
#   # Push specific project
#   PROJECT=myapp ./prometheus-pushgateway.sh
#
#   # Push with custom Pushgateway URL
#   PUSHGATEWAY_URL=http://pushgateway.example.com:9091 ./prometheus-pushgateway.sh
#
#   # Push last 7 days only
#   DAYS=7 ./prometheus-pushgateway.sh

set -euo pipefail

# Configuration
PUSHGATEWAY_URL="${PUSHGATEWAY_URL:-http://localhost:9091}"
PROJECT="${1:-${PROJECT:-}}"
DAYS="${DAYS:-30}"

# Validate pushgateway is reachable
if ! curl --fail --silent --head "$PUSHGATEWAY_URL" >/dev/null 2>&1; then
  echo "Error: Pushgateway not reachable at $PUSHGATEWAY_URL" >&2
  echo "Set PUSHGATEWAY_URL environment variable to the correct URL" >&2
  exit 1
fi

# Build claudewatch export command
EXPORT_CMD="claudewatch export --format prometheus --days $DAYS"
if [ -n "$PROJECT" ]; then
  EXPORT_CMD="$EXPORT_CMD --project $PROJECT"
  echo "Exporting metrics for project: $PROJECT (last $DAYS days)"
else
  echo "Exporting metrics for all projects (last $DAYS days)"
fi

# Export and push to Pushgateway
# Use hostname as instance label for multi-host deployments
$EXPORT_CMD | \
  curl --data-binary @- \
       --fail \
       --silent \
       --show-error \
       "$PUSHGATEWAY_URL/metrics/job/claudewatch/instance/$HOSTNAME"

if [ $? -eq 0 ]; then
  echo "✓ Metrics pushed successfully to $PUSHGATEWAY_URL"
else
  echo "✗ Failed to push metrics" >&2
  exit 1
fi
