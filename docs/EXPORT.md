# Metrics Export

## Overview

The `claudewatch export` command outputs aggregated metrics in formats consumable by external observability platforms. This enables team-wide visibility into Claude Code usage patterns, productivity trends, and friction points without sharing sensitive data.

**Use cases:**
- Track team productivity metrics across all developers
- Monitor friction rates and identify systemic issues
- Analyze cost trends and budget allocation
- Create Grafana dashboards for Claude Code usage
- Set up alerts for high friction or cost spikes
- Compare project health across multiple codebases

**Key principles:**
- **Opt-in only**: Explicit command invocation required
- **Aggregates only**: No transcript content, file paths, or code
- **Local control**: Output to stdout, you decide the destination
- **No network by default**: Everything stays on your machine until you pipe it elsewhere

## Quick Start

### Basic Export

Export metrics to stdout in Prometheus format:

```bash
claudewatch export --format prometheus
```

### Filter to Specific Project

Export metrics for a single project:

```bash
claudewatch export --format prometheus --project myapp
```

### Time Window

Export metrics from the last 7 days:

```bash
claudewatch export --format prometheus --days 7
```

### Output to File

Write metrics to a file instead of stdout:

```bash
claudewatch export --format prometheus --output /tmp/metrics.prom
```

## Privacy & Safety

### What IS Exported (Safe)

The export feature outputs **only safe aggregates** that cannot be used to reconstruct sensitive information:

- **Session counts**: Total number of sessions per project
- **Duration aggregates**: Total and average session duration
- **Friction rates**: Percentage of sessions with friction events
- **Friction type counts**: Aggregated counts by friction type (top 10)
- **Tool error averages**: Mean tool errors per session
- **Commit counts**: Total commits and averages per session
- **Cost totals**: Total USD spent, average per session and per commit
- **Agent success rates**: Percentage of successful agent tasks
- **Model usage percentages**: Which models were used (not token counts)
- **Context pressure**: Average context window utilization (0.0-1.0)
- **Project identifiers**: Project name or hash (not absolute paths)

### What is NEVER Exported (Sensitive)

The following data types are **explicitly excluded** to protect privacy:

- **Transcript content**: User messages, assistant responses
- **File paths**: Absolute or relative paths to source files
- **File contents**: Code from Read/Edit/Write tool calls
- **Tool results**: Payloads returned by tool invocations
- **API keys**: Credentials or authentication tokens
- **Session IDs**: UUIDs or timestamps that could correlate with logs
- **Per-message token counts**: Granular usage data
- **User identifiers**: Usernames, email addresses, or system paths

### Privacy Validation

Every export is validated by comprehensive integration tests that check for:
- No absolute file paths (Unix or Windows)
- No session ID patterns (UUIDs, timestamps)
- No API key patterns
- No transcript content markers
- No source code fragments

See `internal/export/integration_test.go` for the complete privacy test suite.

## Command Reference

### `claudewatch export`

Export aggregated metrics to external observability platforms.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `prometheus` | Export format (currently only `prometheus` supported) |
| `--project` | string | `""` (all) | Filter to specific project name |
| `--days` | int | `30` | Time window in days (0 = all time) |
| `--output` | string | `""` (stdout) | Output file path |

**Examples:**

```bash
# Export all projects, last 30 days, to stdout
claudewatch export --format prometheus

# Export single project, last 7 days, to file
claudewatch export --format prometheus \
  --project myapp \
  --days 7 \
  --output /var/lib/node_exporter/claudewatch.prom

# Export all-time data for all projects
claudewatch export --format prometheus --days 0

# Pipe to Prometheus Pushgateway
claudewatch export --format prometheus --project api-server | \
  curl --data-binary @- http://localhost:9091/metrics/job/claudewatch
```

## Integrations

### Prometheus Pushgateway

Push metrics to a Prometheus Pushgateway for ephemeral jobs:

```bash
#!/bin/bash
# push-claudewatch-metrics.sh
set -euo pipefail

PUSHGATEWAY_URL="${PUSHGATEWAY_URL:-http://localhost:9091}"
PROJECT="${PROJECT:-all}"

claudewatch export --format prometheus --project "$PROJECT" | \
  curl --data-binary @- \
       --fail \
       --silent \
       --show-error \
       "$PUSHGATEWAY_URL/metrics/job/claudewatch/instance/$HOSTNAME"

echo "Metrics pushed to $PUSHGATEWAY_URL"
```

Run this script via cron for regular metric collection:

```cron
# Push metrics every hour
0 * * * * /usr/local/bin/push-claudewatch-metrics.sh
```

### Prometheus Node Exporter (Textfile Collector)

Write metrics to the node exporter textfile collector directory for automatic scraping:

```bash
#!/bin/bash
# update-claudewatch-metrics.sh
set -euo pipefail

TEXTFILE_DIR="${TEXTFILE_DIR:-/var/lib/node_exporter/textfile_collector}"
OUTPUT_FILE="$TEXTFILE_DIR/claudewatch.prom"

# Write to temp file first (atomic update)
TMP_FILE=$(mktemp)
trap "rm -f $TMP_FILE" EXIT

claudewatch export --format prometheus > "$TMP_FILE"
mv "$TMP_FILE" "$OUTPUT_FILE"

echo "Metrics written to $OUTPUT_FILE"
```

Configure node exporter to collect from this directory:

```bash
node_exporter --collector.textfile.directory=/var/lib/node_exporter/textfile_collector
```

Set up a systemd timer for automatic updates:

```ini
# /etc/systemd/system/claudewatch-export.timer
[Unit]
Description=Update claudewatch metrics every hour

[Timer]
OnBootSec=5min
OnUnitActiveSec=1h

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/claudewatch-export.service
[Unit]
Description=Export claudewatch metrics

[Service]
Type=oneshot
ExecStart=/usr/local/bin/update-claudewatch-metrics.sh
User=prometheus
```

### Grafana Dashboards

Once metrics are in Prometheus, create Grafana dashboards to visualize trends.

**Example queries:**

```promql
# Sessions per day (rate over 24h)
rate(claudewatch_sessions_total{project="myapp"}[24h]) * 86400

# Friction rate over time
claudewatch_friction_rate{project="myapp"}

# Cost per commit trend
claudewatch_cost_per_commit_avg{project="myapp"}

# Most common friction types (top 5)
topk(5, sum by (type) (claudewatch_friction_events_total))

# Agent success rate across all projects
avg(claudewatch_agent_success_rate)

# Daily cost burn rate
rate(claudewatch_cost_usd_total{project="myapp"}[24h]) * 86400
```

**Sample dashboard panels:**

1. **Productivity Overview**
   - Sessions per day (timeseries)
   - Commits per session (gauge)
   - Zero-commit rate (stat)

2. **Friction Analysis**
   - Friction rate over time (timeseries)
   - Top friction types (bar chart)
   - Tool errors per session (gauge)

3. **Cost Analysis**
   - Daily cost (timeseries)
   - Cost per commit (gauge)
   - Total spend (stat)

4. **Agent Performance**
   - Agent success rate (gauge)
   - Agent usage rate (gauge)
   - Sessions with agents (timeseries)

### Datadog (via StatsD Exporter)

Use the [Prometheus StatsD exporter](https://github.com/prometheus/statsd_exporter) to forward metrics to Datadog:

```bash
# Export to Prometheus format, then forward via StatsD exporter
claudewatch export --format prometheus | \
  statsd_exporter --statsd.mapping-config=/etc/statsd-mapping.yml
```

Configure Datadog agent to scrape the StatsD exporter endpoint.

### AWS CloudWatch

Export to CloudWatch using the [CloudWatch exporter](https://github.com/prometheus/cloudwatch_exporter):

```bash
# Export to file, then use CloudWatch exporter
claudewatch export --format prometheus --output /tmp/metrics.prom

# Configure CloudWatch exporter to read from this file
cloudwatch_exporter --config.file=/etc/cloudwatch-config.yml
```

## Metrics Reference

### Session Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_sessions_total` | counter | Total number of Claude Code sessions | `project` |
| `claudewatch_session_duration_minutes_total` | counter | Total duration of all sessions in minutes | `project` |
| `claudewatch_session_duration_minutes_avg` | gauge | Average session duration in minutes | `project` |
| `claudewatch_active_minutes_total` | counter | Total active minutes (excludes idle time) | `project` |

### Friction Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_friction_rate` | gauge | Fraction of sessions with friction events (0.0-1.0) | `project` |
| `claudewatch_friction_events_total` | counter | Total friction events by type (top 10 types) | `project`, `type` |
| `claudewatch_tool_errors_avg` | gauge | Average tool errors per session | `project` |

**Common friction types:**
- `retry:Bash` - Repeated Bash command failures
- `buggy_code` - Code that didn't work on first attempt
- `excessive_analysis` - Over-exploration before implementation
- `user_rejected_action` - User interrupted or corrected agent
- `permission_denied` - File or system permission errors

### Productivity Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_commits_total` | counter | Total number of git commits created | `project` |
| `claudewatch_commits_per_session_avg` | gauge | Average commits per session | `project` |
| `claudewatch_commit_attempt_ratio` | gauge | Ratio of commits to code change attempts (Edit+Write) | `project` |
| `claudewatch_zero_commit_rate` | gauge | Fraction of sessions with zero commits (0.0-1.0) | `project` |

### Cost Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_cost_usd_total` | counter | Total cost in USD | `project` |
| `claudewatch_cost_per_session_avg` | gauge | Average cost per session in USD | `project` |
| `claudewatch_cost_per_commit_avg` | gauge | Average cost per commit in USD | `project` |

### Model Usage

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_model_usage_percent` | gauge | Percentage of sessions using this model (0-100) | `project`, `model` |

**Common model names:**
- `sonnet` - Claude Sonnet (most common)
- `opus` - Claude Opus (highest quality)
- `haiku` - Claude Haiku (fastest)

**Note:** Top 5 models only to prevent label cardinality explosion.

### Agent Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_agent_success_rate` | gauge | Agent task success rate (0.0-1.0) | `project` |
| `claudewatch_agent_usage_rate` | gauge | Fraction of sessions using agents (0.0-1.0) | `project` |

### Context Pressure

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `claudewatch_context_pressure_avg` | gauge | Average context window utilization (0.0-1.0) | `project` |

**Interpretation:**
- `< 0.5` - Comfortable, plenty of context remaining
- `0.5-0.7` - Filling, monitor for context pressure
- `0.7-0.9` - Pressure, consider compaction
- `> 0.9` - Critical, compaction needed

## Troubleshooting

### No metrics output

**Problem:** `claudewatch export` produces empty output or only headers.

**Solutions:**

1. Check if sessions exist:
   ```bash
   claudewatch metrics --days 30
   ```

2. Verify project name filter:
   ```bash
   # List all sessions to find correct project names
   ls ~/.claude/sessions/

   # Try without project filter
   claudewatch export --format prometheus
   ```

3. Check time window:
   ```bash
   # Try all-time export
   claudewatch export --format prometheus --days 0
   ```

### "Unsupported format" error

**Problem:** `Error: unsupported format: json`

**Solution:** Currently only Prometheus format is supported. JSON and other formats are planned for future releases.

```bash
# Use this:
claudewatch export --format prometheus

# Not this:
claudewatch export --format json  # Not implemented yet
```

### High cardinality warnings

**Problem:** Too many unique label values in Prometheus, causing memory issues.

**Solution:** The exporter automatically limits cardinality:
- Friction types: Top 10 only
- Model names: Top 5 only

If you still see issues, filter to specific projects:

```bash
# Instead of exporting all projects at once
claudewatch export --format prometheus

# Export each project individually
for project in api backend frontend; do
  claudewatch export --format prometheus --project "$project" \
    --output "/var/lib/node_exporter/claudewatch-${project}.prom"
done
```

### Metrics not updating in Prometheus

**Problem:** Prometheus shows stale metrics or no new data.

**Debugging steps:**

1. Verify export generates fresh data:
   ```bash
   claudewatch export --format prometheus | head -20
   ```

2. Check file timestamps (for textfile collector):
   ```bash
   ls -l /var/lib/node_exporter/textfile_collector/claudewatch.prom
   ```

3. Verify Prometheus scrape config:
   ```bash
   # For Pushgateway
   curl http://localhost:9091/metrics | grep claudewatch

   # For textfile collector
   curl http://localhost:9100/metrics | grep claudewatch
   ```

4. Check Prometheus logs for scrape errors:
   ```bash
   journalctl -u prometheus -f
   ```

### Permission denied writing output file

**Problem:** `Error: failed to write output file: permission denied`

**Solution:** Ensure the output directory is writable:

```bash
# Check permissions
ls -ld /var/lib/node_exporter/textfile_collector/

# Fix if needed (run as root)
sudo chown prometheus:prometheus /var/lib/node_exporter/textfile_collector/
sudo chmod 755 /var/lib/node_exporter/textfile_collector/
```

Or write to a temp location and move with elevated privileges:

```bash
claudewatch export --format prometheus --output /tmp/metrics.prom
sudo mv /tmp/metrics.prom /var/lib/node_exporter/textfile_collector/claudewatch.prom
```

## Best Practices

### 1. Regular Export Schedule

Set up automated exports via cron or systemd timers:

```bash
# Hourly export (good for active projects)
0 * * * * claudewatch export --format prometheus --output /tmp/metrics.prom

# Daily export (good for team dashboards)
0 0 * * * claudewatch export --format prometheus --project myapp >> /var/log/claudewatch-export.log 2>&1
```

### 2. Per-Project Export

For large teams, export each project separately to avoid cardinality issues:

```bash
#!/bin/bash
# export-all-projects.sh
for project in $(claudewatch projects --list); do
  claudewatch export --format prometheus \
    --project "$project" \
    --output "/var/lib/node_exporter/claudewatch-${project}.prom"
done
```

### 3. Monitoring Setup

Set up alerts for key metrics:

```yaml
# prometheus-alerts.yml
groups:
  - name: claudewatch
    interval: 5m
    rules:
      - alert: HighFrictionRate
        expr: claudewatch_friction_rate > 0.5
        for: 24h
        annotations:
          summary: "High friction rate detected"
          description: "Project {{ $labels.project }} has {{ $value }}% friction rate"

      - alert: HighCostBurn
        expr: rate(claudewatch_cost_usd_total[24h]) * 86400 > 50
        for: 1h
        annotations:
          summary: "High daily cost detected"
          description: "Project {{ $labels.project }} is burning ${{ $value }}/day"

      - alert: LowAgentSuccess
        expr: claudewatch_agent_success_rate < 0.7
        for: 7d
        annotations:
          summary: "Low agent success rate"
          description: "Project {{ $labels.project }} has {{ $value }}% agent success"
```

### 4. Privacy-Conscious Export

Always verify no sensitive data before sharing:

```bash
# Export to temp file first
claudewatch export --format prometheus --output /tmp/metrics.prom

# Review output manually
less /tmp/metrics.prom

# Check for sensitive patterns
grep -E "(sk-|/Users/|/home/|C:\\\\)" /tmp/metrics.prom

# If clean, push to remote
curl --data-binary @/tmp/metrics.prom http://pushgateway:9091/metrics/job/claudewatch
```

### 5. Cost Tracking

Export cost metrics separately for budget monitoring:

```bash
# Daily cost report
claudewatch export --format prometheus --days 1 | \
  grep claudewatch_cost_usd_total | \
  awk '{print "Daily cost: $" $NF}'

# Weekly cost trend
for i in {7..0}; do
  date -d "$i days ago" +%Y-%m-%d
  claudewatch export --format prometheus --days 1 | \
    grep claudewatch_cost_usd_total | \
    awk '{print $NF}'
done
```

## Future Enhancements

The following features are planned for future releases:

### Phase 2: Additional Formats

- **JSON export**: For generic HTTP endpoints and custom integrations
- **CSV export**: For spreadsheet analysis and reporting
- **StatsD export**: Native Datadog and CloudWatch integration

### Phase 3: Daemon Mode

- **Continuous export**: `claudewatch export --daemon --endpoint <url>`
- **Built-in authentication**: Support for common platform auth methods
- **Rate limiting**: Automatic throttling for large datasets
- **Buffering**: Batch exports for efficiency

### Phase 4: Advanced Filtering

- **Date range filters**: `--since YYYY-MM-DD --until YYYY-MM-DD`
- **Model filters**: `--model sonnet` to export specific model usage
- **User filters**: `--user alice` for multi-user systems
- **Custom aggregations**: User-defined metric calculations

## Related Documentation

- [README.md](../README.md) - Overview and installation
- [internal/export/](../internal/export/) - Implementation details
- [examples/](../examples/) - Integration scripts
- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/)
- [Grafana Documentation](https://grafana.com/docs/)

## Support

For issues or questions:

1. Check [Troubleshooting](#troubleshooting) section above
2. Review [integration tests](../internal/export/integration_test.go) for usage examples
3. Open an issue on GitHub with:
   - Output of `claudewatch export --format prometheus | head -50`
   - Your use case and desired output
   - Any error messages or unexpected behavior

## License

This feature is part of claudewatch and licensed under the same terms as the main project.
