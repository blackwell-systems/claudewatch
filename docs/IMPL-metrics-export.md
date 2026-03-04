# IMPL: Metrics Export to External Observability Platforms

**Status:** SUITABLE for SAW parallelization
**Verdict:** This feature cleanly decomposes into 2-3 independent agents with disjoint file ownership and minimal investigation overhead.

---

## Suitability Gate (5 Questions)

### 1. Can this decompose into ≥2 agents with disjoint file ownership?

**YES** - Clear decomposition into 3 agents:

- **Agent A: Exporter Interface + Prometheus** - Create exporter abstraction, implement Prometheus text format, CLI command
- **Agent B: Metrics Collector** - Build safe metric aggregation layer that pulls from existing analyzer package
- **Agent C: Integration Tests + Documentation** - End-to-end validation, examples, README updates

File ownership is naturally disjoint:
- Agent A: `internal/export/` package (new), `internal/app/export.go` (new)
- Agent B: `internal/export/metrics.go` (new), integration with `internal/analyzer/`
- Agent C: `internal/export/export_test.go` (new), `docs/EXPORT.md` (new), `README.md` (existing)

### 2. Are interfaces definable upfront without investigation?

**YES** - The interface contract is straightforward:

```go
// Exporter writes metrics in a specific format
type Exporter interface {
    Export(metrics MetricSnapshot) ([]byte, error)
    Format() string // "prometheus", "json", etc.
}

// MetricSnapshot is the safe, aggregated data to export
type MetricSnapshot struct {
    Timestamp      time.Time
    ProjectName    string
    SessionCount   int
    FrictionRate   float64
    FrictionByType map[string]int
    // ... (see detailed spec below)
}
```

No investigation needed - the metrics already exist in `internal/analyzer/`, we just need a safe aggregation wrapper.

### 3. Can agents work in parallel with minimal coordination?

**YES** - Agents have sequential checkpoints but parallelizable work:

**Wave 1** (parallel after SCOUT writes interfaces):
- Agent A implements Prometheus exporter + CLI skeleton
- Agent B implements MetricSnapshot collector

**Wave 2** (after Wave 1 completes):
- Agent C writes integration tests using both A and B's outputs

Coordination points:
1. SCOUT defines `Exporter` interface and `MetricSnapshot` struct in IMPL doc
2. Agent A/B work in parallel referencing the interface contract
3. Agent C validates integration

### 4. What's the investigation overhead?

**LOW** (≤30 min) - Minimal unknowns:

**Known:**
- Metrics already computed in `internal/analyzer/` (seen in `metrics.go`)
- CLI command pattern established (Cobra in `internal/app/`)
- Output formatting patterns exist (`internal/output/`)

**Minor investigation needed:**
- Prometheus text format spec (well-documented standard)
- Which specific metrics to prioritize (user already provided list)
- Safe default filtering rules (no sensitive data)

**No investigation needed:**
- Codebase architecture (already explored)
- Data access patterns (analyzer package is clean)
- Testing patterns (test files exist throughout)

### 5. What's the parallelization value?

**HIGH** - 3 agents working simultaneously reduces wall-clock time from ~90 min (serial) to ~40 min (parallel).

**Serial estimate:** 30min (interface) + 30min (Prometheus) + 30min (collector) + 20min (tests) = ~110min
**Parallel estimate:** 30min (SCOUT) + 30min (Wave 1: A+B parallel) + 20min (Wave 2: C) = ~80min
**Savings:** ~30% time reduction + reduced context loss

---

## Verdict: SUITABLE ✓

This feature meets all SAW suitability criteria:
- Clean decomposition with disjoint file ownership
- Upfront interface design is straightforward
- Minimal investigation overhead
- High parallelization value
- No complex unknowns requiring serial exploration

Proceed with full IMPL documentation below.

---

## Feature Overview

Add `claudewatch export` command to output aggregated metrics in formats consumable by external observability platforms. Phase 1 focuses on Prometheus text format with stdout output (users pipe to their destination).

**Privacy principles:**
- Opt-in only (explicit command invocation)
- Aggregates only (no transcript content, file paths, or code)
- Local control (stdout → user decides destination)
- No network by default

---

## Architecture

### Package Structure

```
internal/export/
├── exporter.go       # Exporter interface + registry
├── prometheus.go     # PrometheusExporter implementation
├── metrics.go        # MetricSnapshot type + safe collector
├── export_test.go    # Unit tests
└── integration_test.go # End-to-end tests

internal/app/
└── export.go         # CLI command (new)
```

### Core Types

```go
// Package export provides metrics export capabilities.
package export

import "time"

// Exporter formats metrics for external platforms.
type Exporter interface {
    // Export renders a MetricSnapshot in the exporter's format.
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
    SessionCount        int
    TotalDurationMin    float64
    AvgDurationMin      float64
    ActiveMinutes       float64

    // Friction metrics
    FrictionRate        float64  // sessions with friction / total sessions
    FrictionByType      map[string]int
    AvgToolErrors       float64

    // Productivity metrics
    TotalCommits        int
    AvgCommitsPerSession float64
    CommitAttemptRatio  float64  // commits / (Edit+Write tool uses)
    ZeroCommitRate      float64

    // Cost metrics (USD)
    TotalCostUSD        float64
    AvgCostPerSession   float64
    CostPerCommit       float64

    // Model usage (percentages, not token counts)
    ModelUsagePct       map[string]float64  // model name → % of sessions

    // Agent metrics
    AgentSuccessRate    float64
    AgentUsageRate      float64  // sessions with agents / total

    // Context pressure (aggregated status)
    AvgContextPressure  float64  // 0.0-1.0
}
```

---

## Agent Decomposition

### Agent A: Exporter Interface + Prometheus Implementation

**Files to create:**
- `internal/export/exporter.go` - Interface definition + registry
- `internal/export/prometheus.go` - Prometheus text format implementation
- `internal/app/export.go` - CLI command skeleton

**Responsibilities:**
1. Define `Exporter` interface
2. Implement `PrometheusExporter` that outputs Prometheus text format
3. Create `claudewatch export --format prometheus` CLI command
4. Handle stdout/file output modes

**Interface:**
```go
// Exporter registry
var exporters = map[string]Exporter{
    "prometheus": &PrometheusExporter{},
}

func GetExporter(format string) (Exporter, error) {
    e, ok := exporters[format]
    if !ok {
        return nil, fmt.Errorf("unsupported format: %s", format)
    }
    return e, nil
}
```

**Prometheus format example:**
```
# HELP claudewatch_sessions_total Total number of Claude Code sessions
# TYPE claudewatch_sessions_total counter
claudewatch_sessions_total{project="claudewatch"} 42

# HELP claudewatch_friction_rate Fraction of sessions with friction events
# TYPE claudewatch_friction_rate gauge
claudewatch_friction_rate{project="claudewatch"} 0.35

# HELP claudewatch_cost_usd_total Total cost in USD
# TYPE claudewatch_cost_usd_total counter
claudewatch_cost_usd_total{project="claudewatch"} 12.45
```

**CLI command:**
```bash
# Output to stdout (default)
claudewatch export --format prometheus

# Filter to specific project
claudewatch export --format prometheus --project claudewatch

# Time window
claudewatch export --format prometheus --days 7

# Output to file
claudewatch export --format prometheus --output /tmp/metrics.txt
```

**Dependencies:**
- None (creates new interface)
- Minimal imports: `fmt`, `bytes`, `strings`, `time`

**Exit criteria:**
- `Exporter` interface defined in `internal/export/exporter.go`
- `PrometheusExporter` implementation complete
- CLI command `claudewatch export --format prometheus` outputs valid Prometheus text format
- Unit tests pass for Prometheus formatter

---

### Agent B: Metrics Collector

**Files to create:**
- `internal/export/metrics.go` - MetricSnapshot type + CollectMetrics function

**Files to read (not modify):**
- `internal/analyzer/*.go` - Extract safe aggregates from existing analyzers
- `internal/claude/session.go` - Understand SessionMeta structure
- `internal/store/*.go` - Understand data access patterns

**Responsibilities:**
1. Define `MetricSnapshot` struct (following privacy principles)
2. Implement `CollectMetrics(cfg, project, days) (MetricSnapshot, error)` function
3. Pull data from `internal/analyzer` package (reuse existing computation)
4. Apply safety filters (no sensitive data leakage)

**Function signature:**
```go
// CollectMetrics gathers safe, aggregated metrics for export.
func CollectMetrics(cfg *config.Config, projectFilter string, days int) (MetricSnapshot, error) {
    // Load sessions, facets, agent tasks
    // Filter by project and time window
    // Call existing analyzer functions
    // Build MetricSnapshot with safe aggregates only
    return MetricSnapshot{...}, nil
}
```

**Safety rules:**
1. Never include transcript content
2. Never include absolute file paths (use project hash/name only)
3. Never include user messages or tool results
4. Only export aggregated counts, rates, percentages
5. Cost data: totals only, no per-message breakdown

**Data sources:**
- `internal/analyzer.AnalyzeVelocity()` → session count, duration
- `internal/analyzer.AnalyzeFrictionPersistence()` → friction rate by type
- `internal/analyzer.AnalyzeEfficiency()` → tool error rates
- `internal/analyzer.AnalyzeCommits()` → commit metrics
- `internal/analyzer.AnalyzeOutcomes()` → cost metrics
- `internal/analyzer.AnalyzeAgents()` → agent success rate

**Dependencies:**
- Reads from `internal/analyzer` (no modification)
- Reads from `internal/claude` (no modification)
- Independent of Agent A (no file conflicts)

**Exit criteria:**
- `MetricSnapshot` struct defined with all fields documented
- `CollectMetrics()` function implemented and tested
- Unit tests validate privacy rules (no sensitive data in output)
- Integration with existing analyzer package works correctly

---

### Agent C: Integration Tests + Documentation

**Files to create:**
- `internal/export/integration_test.go` - End-to-end validation
- `docs/EXPORT.md` - Export feature documentation
- `examples/prometheus-pushgateway.sh` - Example integration script

**Files to modify:**
- `README.md` - Add "Metrics Export" section

**Responsibilities:**
1. Write end-to-end test: collect metrics → export to Prometheus → validate format
2. Document export feature usage patterns
3. Provide example integration scripts (Prometheus pushgateway, node exporter)
4. Update README with export capabilities

**Integration test structure:**
```go
func TestExportPrometheus_EndToEnd(t *testing.T) {
    // 1. Set up test fixtures (mock sessions, facets)
    // 2. Call CollectMetrics()
    // 3. Export to Prometheus format
    // 4. Validate output:
    //    - Valid Prometheus text format
    //    - Contains expected metrics
    //    - No sensitive data leaked
    // 5. Test CLI command directly
}
```

**Example scripts:**
```bash
# Push to Prometheus Pushgateway
#!/bin/bash
claudewatch export --format prometheus --project myapp | \
  curl --data-binary @- http://localhost:9091/metrics/job/claudewatch

# Write to node exporter textfile collector
claudewatch export --format prometheus --output /var/lib/node_exporter/claudewatch.prom
```

**Documentation sections:**
1. **Overview** - What metrics export does, why it's useful
2. **Privacy** - What data is/isn't exported
3. **Usage** - CLI examples
4. **Integrations** - Prometheus, Datadog, CloudWatch examples
5. **Metrics Reference** - List of exported metrics with descriptions

**Dependencies:**
- Requires Agent A + B to be complete (uses both outputs)
- No file conflicts (only creates new files + README append)

**Exit criteria:**
- Integration test passes end-to-end
- `docs/EXPORT.md` complete with usage examples
- README updated with export feature section
- Example scripts tested manually

---

## Interfaces (Orchestrator defines these upfront)

### 1. Exporter Interface

Defined by Agent A, consumed by CLI and future exporter implementations.

```go
// Exporter formats metrics for external platforms.
type Exporter interface {
    // Export renders a MetricSnapshot in the exporter's format.
    // Returns formatted output suitable for stdout or file write.
    Export(snapshot MetricSnapshot) ([]byte, error)

    // Format returns the format identifier (e.g., "prometheus", "json").
    Format() string
}
```

### 2. MetricSnapshot Structure

Defined by Agent B, consumed by Agent A's exporters.

```go
// MetricSnapshot contains safe, aggregated metrics for export.
type MetricSnapshot struct {
    Timestamp           time.Time
    ProjectName         string
    ProjectHash         string
    SessionCount        int
    TotalDurationMin    float64
    AvgDurationMin      float64
    ActiveMinutes       float64
    FrictionRate        float64
    FrictionByType      map[string]int
    AvgToolErrors       float64
    TotalCommits        int
    AvgCommitsPerSession float64
    CommitAttemptRatio  float64
    ZeroCommitRate      float64
    TotalCostUSD        float64
    AvgCostPerSession   float64
    CostPerCommit       float64
    ModelUsagePct       map[string]float64
    AgentSuccessRate    float64
    AgentUsageRate      float64
    AvgContextPressure  float64
}
```

### 3. CollectMetrics Function

Defined by Agent B, consumed by CLI command.

```go
// CollectMetrics gathers safe, aggregated metrics for export.
// projectFilter: empty string = all projects, or specific project name
// days: time window (0 = all time)
func CollectMetrics(cfg *config.Config, projectFilter string, days int) (MetricSnapshot, error)
```

---

## Wave Execution Plan

### Wave 0: Scout (30 min)

**Output:** This IMPL document written to `docs/IMPL-metrics-export.md`

**Orchestrator writes:**
- Interface definitions above
- Agent task descriptions
- Exit criteria for each agent

### Wave 1: Parallel Implementation (30 min)

**Agent A:** Exporter Interface + Prometheus
- `internal/export/exporter.go`
- `internal/export/prometheus.go`
- `internal/app/export.go`

**Agent B:** Metrics Collector
- `internal/export/metrics.go`

**Synchronization:** Both agents reference interface contracts from IMPL doc (no file conflicts)

### Wave 2: Integration (20 min)

**Agent C:** Tests + Documentation
- `internal/export/integration_test.go`
- `docs/EXPORT.md`
- `examples/prometheus-pushgateway.sh`
- `README.md` update

**Synchronization:** Runs after Wave 1 completes (needs both A and B outputs)

### Wave 3: Validation & User Handoff (10 min)

**Orchestrator:**
1. Run full test suite: `go test ./internal/export/...`
2. Validate CLI: `go run cmd/claudewatch/main.go export --format prometheus --project claudewatch`
3. Check Prometheus format with validator (if available)
4. Review documentation completeness
5. Report results to user

---

## Testing Strategy

### Unit Tests (per agent)

**Agent A - Prometheus Exporter:**
- `TestPrometheusExporter_Export` - Valid format output
- `TestPrometheusExporter_EscapeLabelValues` - Special character handling
- `TestPrometheusExporter_EmptySnapshot` - Graceful handling of zero metrics

**Agent B - Metrics Collector:**
- `TestCollectMetrics_AllProjects` - Aggregates across all projects
- `TestCollectMetrics_SingleProject` - Filters to one project correctly
- `TestCollectMetrics_TimeWindow` - Respects days filter
- `TestCollectMetrics_PrivacyRules` - No sensitive data in output

**Agent C - Integration:**
- `TestExportCLI_Prometheus` - End-to-end CLI execution
- `TestExportCLI_FileOutput` - Writes to specified file
- `TestExportCLI_InvalidFormat` - Error handling

### Manual Validation (Orchestrator)

```bash
# Generate sample output
claudewatch export --format prometheus --project claudewatch

# Validate format (if promtool available)
claudewatch export --format prometheus | promtool check metrics

# Test with actual Prometheus
docker run -p 9090:9090 -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
claudewatch export --format prometheus --output /tmp/metrics.txt
# Configure Prometheus to scrape /tmp/metrics.txt
```

---

## Privacy & Safety Checklist

**Exported (safe):**
- ✓ Session counts
- ✓ Duration aggregates (total, average)
- ✓ Friction rates (percentages)
- ✓ Tool error counts (aggregated)
- ✓ Commit counts
- ✓ Cost totals (USD, no content)
- ✓ Agent success rates (percentages)
- ✓ Model usage percentages
- ✓ Project name or hash (not absolute paths)

**Never exported (sensitive):**
- ✗ Transcript content (user messages, assistant responses)
- ✗ File paths (absolute or relative)
- ✗ File contents from Read/Edit tools
- ✗ Tool result payloads
- ✗ API keys or credentials
- ✗ Per-message token counts
- ✗ Session IDs (could be used to correlate with logs)

**Implementation validation:**
```go
func TestPrivacyRules(t *testing.T) {
    snapshot := CollectMetrics(cfg, "", 30)
    output, _ := PrometheusExporter{}.Export(snapshot)

    // Assert no sensitive data patterns
    require.NotContains(t, output, "/Users/")  // No absolute paths
    require.NotContains(t, output, "session_")  // No session IDs
    require.NotContains(t, output, "sk-")       // No API keys
}
```

---

## Future Extensions (Out of Scope for Phase 1)

**Phase 2: Daemon Mode**
- `claudewatch export --daemon --endpoint <url>` for continuous push
- Built-in auth helpers for common platforms
- Rate limiting and buffering

**Phase 3: Multi-Format Support**
- JSON exporter (for generic HTTP endpoints)
- StatsD exporter (for Datadog)
- CloudWatch exporter (AWS)

**Phase 4: Plugin Architecture**
- Config-driven exporter selection
- User-defined custom exporters
- Webhook support

---

## Open Questions (Minimal - answers provided)

1. **Prometheus metric naming convention?**
   - Use `claudewatch_` prefix for all metrics
   - Follow Prometheus naming best practices: `<namespace>_<subsystem>_<name>_<unit>`
   - Examples: `claudewatch_sessions_total`, `claudewatch_friction_rate`, `claudewatch_cost_usd_total`

2. **Default time window for export?**
   - Default: 30 days (matches `claudewatch metrics --days 30`)
   - User can override with `--days` flag
   - 0 = all time

3. **Project filtering behavior?**
   - No `--project` flag = aggregate across all projects (add `project="<name>"` label to each metric)
   - With `--project` flag = filter to single project
   - Multi-project support via labels (Prometheus-native)

4. **Metric cardinality limits?**
   - Limit friction type breakdowns to top 10 types (avoid label explosion)
   - Limit model usage to top 5 models
   - Document cardinality limits in `docs/EXPORT.md`

---

## Success Criteria

**Feature complete when:**
1. ✓ `claudewatch export --format prometheus` outputs valid Prometheus text format
2. ✓ Exported metrics match privacy checklist (no sensitive data)
3. ✓ CLI supports `--project`, `--days`, `--output` flags
4. ✓ Unit tests pass for all components (≥80% coverage)
5. ✓ Integration test validates end-to-end flow
6. ✓ `docs/EXPORT.md` documents usage and integrations
7. ✓ README updated with export feature section
8. ✓ Example scripts provided for Prometheus pushgateway

**User can:**
- Run `claudewatch export --format prometheus` and pipe to Prometheus
- Filter metrics by project and time window
- Validate no sensitive data is exposed
- Integrate with standard observability platforms

---

## Estimated Effort

**Total serial time:** ~110 minutes
- Scout (IMPL doc): 30 min
- Agent A (Prometheus exporter): 30 min
- Agent B (Metrics collector): 30 min
- Agent C (Tests + docs): 20 min
- Validation: 10 min

**Total parallel time (SAW):** ~80 minutes
- Wave 0 (Scout): 30 min
- Wave 1 (A+B parallel): 30 min
- Wave 2 (C): 20 min
- Wave 3 (Validation): 10 min

**Time savings:** ~30% reduction via parallelization

---

## Dependencies

**External:**
- None (standard library only for Phase 1)

**Internal:**
- `internal/analyzer` (read-only, no modifications)
- `internal/claude` (read-only, no modifications)
- `internal/config` (read-only, no modifications)
- `internal/app` (add new command file)

**No breaking changes:**
- All new code, no modifications to existing commands
- Backwards compatible (new opt-in feature)

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Privacy leak (sensitive data exported) | Low | High | Comprehensive unit tests + privacy checklist validation |
| Invalid Prometheus format | Low | Medium | Use promtool validator in CI, reference official spec |
| Performance (large datasets) | Medium | Low | Add pagination if needed (Phase 2), document limits |
| Agent coordination failure | Low | Low | Clear interface contracts in IMPL doc, minimal inter-agent dependencies |

---

## Appendix: Prometheus Metric Reference

```
# Session metrics
claudewatch_sessions_total{project="<name>"}                # counter
claudewatch_session_duration_minutes_total{project="<name>"} # counter
claudewatch_session_duration_minutes_avg{project="<name>"}   # gauge

# Friction metrics
claudewatch_friction_rate{project="<name>"}                 # gauge (0.0-1.0)
claudewatch_friction_events_total{project="<name>",type="<type>"} # counter
claudewatch_tool_errors_avg{project="<name>"}              # gauge

# Productivity metrics
claudewatch_commits_total{project="<name>"}                 # counter
claudewatch_commits_per_session_avg{project="<name>"}      # gauge
claudewatch_zero_commit_rate{project="<name>"}             # gauge (0.0-1.0)

# Cost metrics
claudewatch_cost_usd_total{project="<name>"}               # counter
claudewatch_cost_per_session_avg{project="<name>"}         # gauge
claudewatch_cost_per_commit_avg{project="<name>"}          # gauge

# Agent metrics
claudewatch_agent_success_rate{project="<name>"}           # gauge (0.0-1.0)
claudewatch_agent_usage_rate{project="<name>"}             # gauge (0.0-1.0)

# Model usage
claudewatch_model_usage_percent{project="<name>",model="<name>"} # gauge (0-100)

# Context pressure
claudewatch_context_pressure_avg{project="<name>"}         # gauge (0.0-1.0)
```

---

## Wave 1 Execution - Agent A Completion Report

### Agent A - Completion Report

**Status:** complete

**Files Changed:**
- `internal/export/exporter.go` (created) - Exporter interface + registry
- `internal/export/prometheus.go` (created) - PrometheusExporter implementation
- `internal/export/prometheus_test.go` (created) - Comprehensive unit tests
- `internal/export/metrics.go` (created) - CollectMetrics stub for Agent B
- `internal/app/export.go` (created) - CLI command

**Commits:** 9fd77135225f90760f3e6c3a4b577a442de1faeb

**Interface Deviations:** None - implemented exactly as specified in IMPL document

**Downstream Action Required:** false - Agent B can implement CollectMetrics independently

**Notes:**
- All unit tests passing (7/7 tests, 0 failures)
- Code compiles cleanly (go build ./... successful)
- go vet passes with no warnings
- CLI command functional with proper help text
- Prometheus text format follows spec with proper escaping
- Cardinality limits enforced (top 10 friction types, top 5 models)
- Label escaping tested and working correctly
- Created stub for CollectMetrics() that returns error - Agent B will replace
- No dependencies on Agent B's work - interfaces defined and tested with mock data

**Exit Criteria Status:**
- [x] `Exporter` interface defined
- [x] `PrometheusExporter` implements interface
- [x] CLI command exists and compiles
- [x] Unit tests pass for Prometheus formatter
- [x] Output is valid Prometheus text format

---

**IMPL Status:** READY FOR WAVE EXECUTION
**Last Updated:** 2026-03-04
**Author:** Scout (Sonnet 4.5)

---

## Agent B - Completion Report

**Status:** complete
**Files Changed:**
- `internal/export/metrics.go` (created, 475 lines)
- `internal/export/metrics_test.go` (created, 284 lines)
- `go.mod` (updated dependencies)

**Commits:**
- `5927bfc` - feat(export): add metrics collector with privacy-safe aggregation

**Interface Deviations:** none

**Downstream Action Required:** false

**Notes:**
- All 17 unit tests passing
- Privacy validation tests confirm no sensitive data in MetricSnapshot
- Successfully integrates with existing analyzer package functions
- Uses `analyzer.NoCacheRatio()` as default (can be enhanced to load stats-cache later)
- Uses `claude.ParseAgentTasks()` (not ParseAllAgentTasks - correct function name)
- Uses `analyzer.DefaultPricing["sonnet"]` as default model pricing
- `LimitFrictionTypes()` utility provided for Prometheus label cardinality control
- Ready for Agent A to consume via `CollectMetrics()` function
- No modifications to existing analyzer or claude packages (read-only as specified)
