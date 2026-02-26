# claudewatch Implementation Plan

## 1. Project Overview

claudewatch is a standalone Go CLI tool that provides observability and continuous improvement for AI-assisted development workflows. It reads Claude Code's local data files, analyzes session patterns, scores project readiness, surfaces friction, and tracks improvement over time. The improvement loop is: Measure, Analyze, Suggest, Implement, Measure again.

---

## 2. Project Structure

```
claudewatch/
├── cmd/
│   └── claudewatch/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── root.go
│   │   ├── scan.go
│   │   ├── metrics.go
│   │   ├── gaps.go
│   │   ├── suggest.go
│   │   ├── track.go
│   │   └── log.go             # claudewatch log (custom metrics)
│   ├── claude/
│   │   ├── history.go
│   │   ├── stats.go
│   │   ├── settings.go
│   │   ├── session_meta.go
│   │   ├── facets.go
│   │   ├── projects.go
│   │   ├── plugins.go
│   │   ├── commands.go
│   │   ├── agents.go          # agent task output parser
│   │   └── types.go
│   ├── scanner/
│   │   ├── discover.go
│   │   ├── score.go
│   │   └── types.go
│   ├── analyzer/
│   │   ├── friction.go
│   │   ├── velocity.go
│   │   ├── satisfaction.go
│   │   ├── efficiency.go
│   │   ├── agents.go          # agent performance analyzer
│   │   └── types.go
│   ├── suggest/
│   │   ├── engine.go
│   │   ├── rules.go
│   │   ├── ranking.go
│   │   └── types.go
│   ├── store/
│   │   ├── db.go
│   │   ├── snapshots.go
│   │   ├── migrations.go
│   │   └── types.go
│   ├── config/
│   │   ├── config.go
│   │   └── defaults.go
│   └── output/
│       ├── table.go
│       ├── progress.go
│       └── color.go
├── .goreleaser.yml
├── Makefile
├── go.mod
├── go.sum
├── CLAUDE.md
├── README.md
└── PLAN.md
```

**Key design decisions:**

- **Cobra** for CLI framework (consistent with shelfctl)
- **lipgloss** for styled terminal output
- **modernc.org/sqlite** (pure Go SQLite, no CGO) for metrics database — enables CGO_ENABLED=0 cross-compilation
- **No external network calls.** Everything reads local files.

---

## 3. Data Layer

### 3.1 Data Sources and Their Schemas

**`~/.claude/history.jsonl`** — User messages across all sessions.

```go
type HistoryEntry struct {
    Display        string            `json:"display"`
    PastedContents map[string]any    `json:"pastedContents"`
    Timestamp      int64             `json:"timestamp"`
    Project        string            `json:"project"`
    SessionID      string            `json:"sessionId"`
}
```

**`~/.claude/stats-cache.json`** — Aggregate stats.

```go
type StatsCache struct {
    Version             int              `json:"version"`
    LastComputedDate    string           `json:"lastComputedDate"`
    DailyActivity       []DailyActivity  `json:"dailyActivity"`
    DailyModelTokens    []DailyTokens    `json:"dailyModelTokens"`
    ModelUsage          map[string]ModelUsage `json:"modelUsage"`
    TotalSessions       int              `json:"totalSessions"`
    TotalMessages       int              `json:"totalMessages"`
    LongestSession      LongestSession   `json:"longestSession"`
    FirstSessionDate    string           `json:"firstSessionDate"`
    HourCounts          map[string]int   `json:"hourCounts"`
}
```

**`~/.claude/usage-data/session-meta/*.json`** — Per-session metadata (40 files).

```go
type SessionMeta struct {
    SessionID            string            `json:"session_id"`
    ProjectPath          string            `json:"project_path"`
    StartTime            string            `json:"start_time"`
    DurationMinutes      int               `json:"duration_minutes"`
    UserMessageCount     int               `json:"user_message_count"`
    AssistantMessageCount int              `json:"assistant_message_count"`
    ToolCounts           map[string]int    `json:"tool_counts"`
    Languages            map[string]int    `json:"languages"`
    GitCommits           int               `json:"git_commits"`
    GitPushes            int               `json:"git_pushes"`
    InputTokens          int               `json:"input_tokens"`
    OutputTokens         int               `json:"output_tokens"`
    FirstPrompt          string            `json:"first_prompt"`
    UserInterruptions    int               `json:"user_interruptions"`
    ToolErrors           int               `json:"tool_errors"`
    ToolErrorCategories  map[string]int    `json:"tool_error_categories"`
    UsesTaskAgent        bool              `json:"uses_task_agent"`
    UsesMCP              bool              `json:"uses_mcp"`
    UsesWebSearch        bool              `json:"uses_web_search"`
    UsesWebFetch         bool              `json:"uses_web_fetch"`
    LinesAdded           int               `json:"lines_added"`
    LinesRemoved         int               `json:"lines_removed"`
    FilesModified        int               `json:"files_modified"`
}
```

**`~/.claude/usage-data/facets/*.json`** — Qualitative session analysis (11 files).

```go
type SessionFacet struct {
    UnderlyingGoal        string            `json:"underlying_goal"`
    GoalCategories        map[string]int    `json:"goal_categories"`
    Outcome               string            `json:"outcome"`
    UserSatisfactionCounts map[string]int   `json:"user_satisfaction_counts"`
    ClaudeHelpfulness     string            `json:"claude_helpfulness"`
    SessionType           string            `json:"session_type"`
    FrictionCounts        map[string]int    `json:"friction_counts"`
    FrictionDetail        string            `json:"friction_detail"`
    PrimarySuccess        string            `json:"primary_success"`
    BriefSummary          string            `json:"brief_summary"`
    SessionID             string            `json:"session_id"`
}
```

**`~/.claude/settings.json`** — Global hooks, permissions, plugins.

**`~/.claude/commands/*.md`** — Custom slash commands (5 currently).

**`~/code/*/CLAUDE.md`** — Project AI configuration (6 of 41 exist).

**`~/code/*/.git/`** — Git repos for commit velocity.

### 3.2 SQLite Schema

Located at `~/.config/claudewatch/claudewatch.db`.

```sql
CREATE TABLE snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    taken_at    TEXT NOT NULL,
    command     TEXT NOT NULL,
    version     TEXT NOT NULL
);

CREATE TABLE project_scores (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    project     TEXT NOT NULL,
    score       REAL NOT NULL,
    has_claude_md     BOOLEAN NOT NULL,
    has_dot_claude    BOOLEAN NOT NULL,
    has_local_settings BOOLEAN NOT NULL,
    session_count     INTEGER NOT NULL,
    last_session_date TEXT,
    primary_language  TEXT,
    git_commit_30d    INTEGER NOT NULL
);

CREATE TABLE aggregate_metrics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    metric_name TEXT NOT NULL,
    metric_value REAL NOT NULL,
    detail      TEXT
);

CREATE TABLE friction_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    session_id  TEXT NOT NULL,
    friction_type TEXT NOT NULL,
    count       INTEGER NOT NULL,
    detail      TEXT,
    project     TEXT,
    session_date TEXT
);

CREATE TABLE suggestions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    category    TEXT NOT NULL,
    priority    INTEGER NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    impact_score REAL NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open'
);

CREATE TABLE agent_tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id     INTEGER NOT NULL REFERENCES snapshots(id),
    session_id      TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    agent_type      TEXT NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL,
    duration_ms     INTEGER,
    total_tokens    INTEGER,
    tool_uses       INTEGER,
    background      BOOLEAN DEFAULT false,
    needed_correction BOOLEAN DEFAULT false,
    created_at      TEXT NOT NULL
);

-- Custom user-injected metrics
CREATE TABLE custom_metrics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at   TEXT NOT NULL,
    session_id  TEXT,
    project     TEXT,
    metric_name TEXT NOT NULL,
    metric_value REAL,
    tags        TEXT,           -- JSON array of string tags
    note        TEXT
);

CREATE INDEX idx_project_scores_snapshot ON project_scores(snapshot_id);
CREATE INDEX idx_aggregate_snapshot ON aggregate_metrics(snapshot_id);
CREATE INDEX idx_friction_snapshot ON friction_events(snapshot_id);
CREATE INDEX idx_suggestions_status ON suggestions(status);
CREATE INDEX idx_agent_tasks_snapshot ON agent_tasks(snapshot_id);
CREATE INDEX idx_agent_tasks_type ON agent_tasks(agent_type);
CREATE INDEX idx_custom_metrics_name ON custom_metrics(metric_name);
CREATE INDEX idx_custom_metrics_session ON custom_metrics(session_id);
CREATE INDEX idx_custom_metrics_project ON custom_metrics(project);
```

---

## 4. Command Specifications

### 4.1 `claudewatch scan`

Inventory all projects, score each for AI readiness.

**Readiness score algorithm (0-100):**

| Factor | Weight | Scoring |
|--------|--------|---------|
| CLAUDE.md exists | 30 | 30 if present, 0 if absent |
| CLAUDE.md quality | 10 | 0-10 based on file size (>500 bytes = 10) |
| .claude/ directory | 10 | 10 if exists |
| Local settings | 5 | 5 if project has .claude/settings.local.json |
| Session history | 15 | 15 scaled by recency (decays over 30 days) |
| Facets coverage | 10 | 10 if project has facet data |
| Active development | 10 | 0-10 based on commits in last 30 days |
| Hook adoption | 5 | 5 if global hooks configured |
| Plugin usage | 5 | 5 if relevant plugins enabled |

**Flags:** `--path`, `--min-score`, `--json`, `--sort`

### 4.2 `claudewatch metrics`

Parse session data, compute and display trends.

**Computed metrics:**
- Session volume (sessions/day, messages/session, duration)
- Productivity (lines added, commits, files modified per session)
- Efficiency (tool errors, user interruptions, tool call distribution)
- Satisfaction (weighted score from facets)
- Token economics (tokens/session, cache hit ratio)
- Working patterns (peak hours, duration distribution)
- Feature adoption (task agents, MCP, web search, web fetch)
- Agent performance (success rate, cost, parallelization — see Section 16)
- Custom metrics trends (see Section 17)

**Flags:** `--days N`, `--project PATH`, `--json`

### 4.3 `claudewatch gaps`

Surface friction patterns and missing configuration.

**Analysis:**
1. CLAUDE.md gaps — projects with sessions but no CLAUDE.md
2. Recurring friction — friction types in >30% of sessions
3. Missing hooks — compare configured vs recommended hooks
4. Unused skills — skills that exist but appear unused
5. Project-specific friction — cross-reference facets by project

### 4.4 `claudewatch suggest`

Generate actionable, ranked improvement recommendations.

**Built-in rules:**
1. MissingClaudeMD — suggest for projects with sessions but no CLAUDE.md
2. RecurringFriction — suggest interventions for common friction
3. HookGaps — suggest missing hooks
4. UnusedSkills — flag unused skills
5. HighErrorProjects — projects with >2x average tool errors
6. AgentAdoption — suggest task agent patterns if underused
7. InterruptionPattern — suggest CLAUDE.md improvements for high-interrupt projects
8. AgentTypeEffectiveness — flag underperforming agent types
9. ParallelizationOpportunity — flag sequential agents that could be parallel
10. CustomMetricRegression — flag custom metrics trending in wrong direction

**Ranking:** `impact_score = (affected_sessions * frequency * time_saved) / effort_minutes`

**Flags:** `--limit N`, `--category`, `--json`, `--project`

### 4.5 `claudewatch track`

Compare current metrics against most recent snapshot.

1. Run same analysis as scan + metrics + gaps
2. Store as new snapshot in SQLite
3. Load previous snapshot
4. Compute deltas for each metric (including custom metrics)
5. Auto-resolve suggestions whose trigger conditions are no longer true

**Flags:** `--compare N`, `--json`

### 4.6 `claudewatch log`

Inject custom user-defined metrics into the database.

See Section 17 for full specification.

---

## 5. Configuration

`~/.config/claudewatch/config.yaml`:

```yaml
scan_paths:
  - ~/code
claude_home: ~/.claude
active_threshold: 1
weights:
  claude_md_exists: 30
  claude_md_quality: 10
  dot_claude_dir: 10
  local_settings: 5
  session_history: 15
  facets_coverage: 10
  active_development: 10
  hook_adoption: 5
  plugin_usage: 5
friction:
  recurring_threshold: 0.30
  high_error_multiplier: 2.0
output:
  color: true
  width: 80

# Custom metric definitions (for claudewatch log)
custom_metrics:
  session_quality:
    type: scale        # scale, boolean, counter, duration
    range: [1, 5]
    direction: higher_is_better
    description: "Overall session quality rating"
  resume_callback:
    type: boolean
    direction: true_is_better
    description: "Did this tailored resume get a callback?"
  time_to_first_commit:
    type: duration
    direction: lower_is_better
    description: "Time from session start to first commit"
  scope_creep:
    type: boolean
    direction: false_is_better
    description: "Did Claude make unrequested changes?"
```

---

## 6. Scoring Algorithm Detail

```go
func ComputeReadiness(p *Project, sessions []SessionMeta, facets []SessionFacet, settings *GlobalSettings) float64 {
    score := 0.0
    if p.HasClaudeMD { score += 30 }
    if p.ClaudeMDSize > 500 { score += 10 } else if p.ClaudeMDSize > 100 { score += 5 }
    if p.HasDotClaude { score += 10 }
    if p.HasLocalSettings { score += 5 }
    projectSessions := filterByProject(sessions, p.Path)
    if len(projectSessions) > 0 {
        score += 15 * recencyWeight(projectSessions[len(projectSessions)-1].StartTime)
    }
    if len(filterFacetsByProject(facets, sessions, p.Path)) > 0 { score += 10 }
    if p.CommitsLast30Days > 20 { score += 10 } else if p.CommitsLast30Days > 5 { score += 5 } else if p.CommitsLast30Days > 0 { score += 2 }
    if len(settings.Hooks) > 0 { score += 5 }
    if hasRelevantPlugin(settings, p.PrimaryLanguage) { score += 5 }
    return score
}
```

---

## 7. Suggest Engine Detail

```go
type Rule func(ctx *AnalysisContext) []Suggestion

type Engine struct { rules []Rule }

func NewEngine() *Engine {
    return &Engine{rules: []Rule{
        MissingClaudeMD, RecurringFriction, HookGaps,
        UnusedSkills, HighErrorProjects, AgentAdoption,
        InterruptionPattern, AgentTypeEffectiveness,
        ParallelizationOpportunity, CustomMetricRegression,
    }}
}
```

Impact scoring: `impact_score = affected_sessions * frequency * time_saved / effort_minutes`

---

## 8. Track System

Snapshot diffing with metric-aware direction (lower friction = improving, higher satisfaction = improving). Auto-resolves suggestions when trigger conditions clear. Custom metrics included in snapshot diffs with user-defined direction awareness.

---

## 9. CLI Framework

Cobra with global flags: `--config`, `--no-color`, `--json`, `--verbose`. Running `claudewatch` with no subcommand shows a quick dashboard.

---

## 10. Testing Strategy

- Unit tests per package with embedded test fixtures
- In-memory SQLite for database tests
- Integration tests with synthetic Claude data in temp directories
- CI: `go test ./...`, `go vet ./...`, `golangci-lint run`

---

## 11. Distribution

- goreleaser (CGO_ENABLED=0, linux/darwin/windows, amd64/arm64)
- Homebrew via `blackwell-systems/homebrew-tap`
- `go install github.com/blackwell-systems/claudewatch/cmd/claudewatch@latest`

---

## 12. Future: CLAUDE.md Draft Generation

`claudewatch draft --project <name>` would detect language, framework, directory structure, and generate a CLAUDE.md template. No API calls — pure template-based generation using project metadata.

---

## 13. Implementation Sequence

### Phase 1: Foundation (Day 1-2)
1. Go module init, Cobra CLI scaffold
2. Config with viper and defaults
3. All data type definitions
4. Parsers for all Claude data sources
5. Parser unit tests

### Phase 2: Scan (Day 2-3)
6. Project discovery (walk directories)
7. Readiness scoring algorithm
8. lipgloss table output
9. Wire up scan command
10. Integration tests

### Phase 3: Metrics + Gaps (Day 3-4)
11. Analyzer modules (friction, velocity, satisfaction, efficiency)
12. Wire up metrics and gaps commands
13. Unit tests

### Phase 4: Suggest + Track + Log (Day 4-5)
14. SQLite store with migrations
15. Suggest engine with rules and ranking
16. Track with snapshot diffing
17. `claudewatch log` command and custom metrics store
18. Unit tests

### Phase 5: Multi-Agent Analytics (Day 5)
19. Agent output parser
20. Agent performance analyzer
21. Agent-specific suggest rules
22. Integration with metrics output

### Phase 6: Polish (Day 5-6)
23. CLAUDE.md for claudewatch itself
24. README.md
25. goreleaser + GitHub Actions CI
26. Makefile
27. End-to-end testing

---

## 14. Dependencies

```
github.com/spf13/cobra       v1.8.x
github.com/spf13/viper       v1.18.x
github.com/charmbracelet/lipgloss v1.1.x
modernc.org/sqlite            latest
```

---

## 15. Key Technical Decisions

- **Pure-Go SQLite** over mattn/go-sqlite3 — no CGO needed, enables cross-compilation
- **No TUI (bubbletea)** — reporting tool, runs and exits. Dashboard mode can be added later
- **Don't parse full session JSONL** — session-meta and facets provide pre-computed summaries. Full transcripts are 70K+ tokens each
- **Store suggestions in SQLite** — enables lifecycle tracking (open → resolved) across snapshots

---

## 16. Multi-Agent Analytics Module

### 16.1 Overview

Multi-agent orchestration is a core part of advanced Claude Code workflows. Tracking agent effectiveness is critical for optimizing when to use agents, which types work best, and whether parallelization is actually saving time.

### 16.2 Data Source

Agent task output files are stored in `/tmp/claude-*/tasks/*.output`. These are JSONL files containing the full agent transcript including timestamps, tool uses, token counts, and completion status.

Each agent task also appears in the parent session's transcript with metadata:
- Agent ID
- Agent type (Explore, Plan, general-purpose, documentation-specialist, etc.)
- Description (3-5 word summary)
- Whether it ran in background
- Completion status and result summary

### 16.3 Agent Metrics Parser

```go
// internal/claude/agents.go

type AgentTask struct {
    AgentID     string `json:"agent_id"`
    AgentType   string `json:"agent_type"`
    Description string `json:"description"`
    SessionID   string `json:"session_id"`
    Status      string `json:"status"` // completed, failed, stopped
    DurationMs  int64  `json:"duration_ms"`
    TotalTokens int    `json:"total_tokens"`
    ToolUses    int    `json:"tool_uses"`
    Background  bool   `json:"background"`
    CreatedAt   string `json:"created_at"`
}
```

Parse agent output files by:
1. Scanning `/tmp/claude-*/tasks/` for `*.output` files
2. Extracting agent metadata from the JSONL entries (type, timestamps, token usage)
3. Cross-referencing with session-meta to link agents to sessions and projects

### 16.4 Computed Agent Metrics

| Metric | Formula | Why it matters |
|--------|---------|----------------|
| Agent success rate | completed without correction / total | Are agents saving time or creating rework? |
| Agent cost per task | total tokens / task count | Token spend vs value delivered |
| Parallelization ratio | max concurrent agents / total agents per session | How well are you leveraging parallelism? |
| Correction rate | tasks needing manual override / total | Which agent types need babysitting? |
| Agent type effectiveness | success rate grouped by agent_type | Explore vs Plan vs general-purpose performance |
| Time saved estimate | (estimated_manual_minutes - agent_duration) per task | ROI on agent delegation |
| Background vs foreground ratio | background / total | Are you using background agents effectively? |

### 16.5 Correction Detection

Detecting whether an agent's output needed correction. Heuristics:

1. **Explicit rejection** — user message after agent completion contains negative keywords ("no", "wrong", "fix", "revert", "undo")
2. **Follow-up edits** — files the agent modified are edited again within 5 minutes
3. **Agent re-runs** — similar agent (same type + similar description) spawned again in same session
4. **Manual override** — user performs the same action the agent was supposed to do

Mark confidence level on each detection.

### 16.6 New Suggest Rules

**AgentTypeEffectiveness:**
```
IF agent_type success_rate < 70% for a given type
THEN suggest: "Your {type} agents succeed only {rate}% of the time.
     Consider breaking complex {type} tasks into smaller, more focused agents."
```

**ParallelizationOpportunity:**
```
IF session has >2 sequential agents that don't depend on each other
THEN suggest: "You ran {n} agents sequentially that could have been parallel,
     costing ~{minutes} extra minutes."
```

**AgentOveruse:**
```
IF simple tasks (< 3 tool uses) are being delegated to agents
THEN suggest: "You're delegating simple tasks to agents that could be done inline.
     Agent overhead (spawn + context) adds ~10s per task."
```

### 16.7 Integration with `claudewatch metrics`

```
 Agent Performance
 ──────────────────────────────────────────────────────────────────
 Total agents spawned   47          (1.2/session)
 Success rate           83%         ▲ +5% vs prior
 Background ratio       68%
 Avg duration           42s
 Avg tokens/agent       12,400

 By type:
  Explore            18  (92% success)  avg 15s   fast, reliable
  general-purpose    14  (71% success)  avg 68s   needs better scoping
  Plan                8  (88% success)  avg 45s
  documentation       7  (86% success)  avg 52s
```

### 16.8 Limitations

- Agent output files in `/tmp/` are ephemeral — lost on reboot. `claudewatch track` should snapshot agent data to preserve it.
- Correction detection is heuristic-based with false positives/negatives.
- Token counts may not perfectly match billing (cache reads, etc.).

---

## 17. Custom Metrics Injection

### 17.1 Overview

Passive observability (parsing Claude's own data) captures what happened mechanically. But there's a whole class of metrics only the human knows: "Was this session actually productive?" "Did that resume get a callback?" "Did Claude's approach match what I would have done?"

Custom metrics close this gap — turning claudewatch from passive monitoring into an active feedback system where human signal compounds with machine data.

### 17.2 The `claudewatch log` Command

```bash
# Log a metric value
claudewatch log session_quality 4
claudewatch log session_quality 4 --session latest
claudewatch log session_quality 4 --session abc123 --note "great flow today"

# Boolean metrics
claudewatch log resume_callback true --project rezmakr
claudewatch log scope_creep false --session latest

# Tags for filtering
claudewatch log session_quality 5 --tag go --tag tui --tag shelfctl

# Counter (increment)
claudewatch log ai_corrections +1 --session latest

# Duration
claudewatch log time_to_first_commit 12m --session latest

# View logged metrics
claudewatch log --list
claudewatch log --list --metric session_quality --days 30
```

### 17.3 Metric Types

Defined in `~/.config/claudewatch/config.yaml` under `custom_metrics`:

```go
type MetricDefinition struct {
    Name        string    `yaml:"name"`
    Type        string    `yaml:"type"`        // scale, boolean, counter, duration
    Range       [2]float64 `yaml:"range"`      // for scale type: [min, max]
    Direction   string    `yaml:"direction"`   // higher_is_better, lower_is_better, true_is_better, false_is_better
    Description string    `yaml:"description"`
}
```

| Type | Input | Stored as | Example |
|------|-------|-----------|---------|
| scale | integer or float | float64 | session_quality: 4 |
| boolean | true/false/yes/no/1/0 | 1.0 or 0.0 | resume_callback: true |
| counter | +N or -N | cumulative float64 | ai_corrections: +1 |
| duration | Ns, Nm, Nh | seconds as float64 | time_to_first_commit: 12m |

### 17.4 Session Resolution

`--session` flag accepts:
- `latest` — most recent session from history.jsonl
- A session ID (UUID)
- Omitted — no session association (project-level or standalone metric)

When `--session latest` is used, claudewatch reads `~/.claude/history.jsonl` to find the most recent session ID.

### 17.5 Integration with Other Commands

**`claudewatch metrics`** — adds a "Custom Metrics" section:

```
 Custom Metrics (last 30 days)
 ──────────────────────────────────────────────────────────────────
 session_quality     avg 3.8  (n=12)     ▲ +0.3 vs prior 30d
 resume_callback     60% true (n=5)      ▲ +20%
 scope_creep         25% true (n=8)      ▼ improved (-15%)
 time_to_first_commit  avg 8m (n=10)     ▲ improved (-3m)
```

**`claudewatch track`** — custom metrics included in snapshot diffs, respecting user-defined direction (lower time_to_first_commit = improving).

**`claudewatch suggest`** — new rule `CustomMetricRegression`:
```
IF custom metric trending in wrong direction for 3+ consecutive snapshots
THEN suggest: "{metric} has gotten worse for 3 consecutive periods.
     Current: {value}, was: {prev_value}. Consider: {contextual_advice}"
```

### 17.6 Hook Integration

Claude Code hooks can auto-log metrics after sessions:

```json
// ~/.claude/settings.json
{
  "hooks": {
    "SessionEnd": [{
      "command": "claudewatch log session_quality --session $CLAUDE_SESSION_ID --interactive"
    }]
  }
}
```

With `--interactive`, claudewatch prompts the user:

```
Session ended (42 min, 8 commits, shelfctl)
Rate this session (1-5): 4
Scope creep? (y/N): n
Note (optional): shipped batch-add feature cleanly
✓ Logged 2 metrics for session abc123
```

This is the **automated feedback loop** — every session ends with a quick self-assessment that feeds back into the improvement system.

### 17.7 Presets

Ship common metric definitions as presets:

```bash
claudewatch log --init-presets
```

Creates default custom_metrics in config:

```yaml
custom_metrics:
  session_quality:
    type: scale
    range: [1, 5]
    direction: higher_is_better
    description: "How productive was this session?"
  session_goal_achieved:
    type: boolean
    direction: true_is_better
    description: "Did you accomplish what you set out to do?"
  scope_creep:
    type: boolean
    direction: false_is_better
    description: "Did Claude make unrequested changes?"
  ai_alignment:
    type: scale
    range: [1, 5]
    direction: higher_is_better
    description: "How well did Claude's approach match your intent?"
  manual_corrections:
    type: counter
    direction: lower_is_better
    description: "Number of times you had to correct Claude"
```

### 17.8 Data Correlation

The power of custom metrics comes from correlating them with passive data:

- "Sessions with CLAUDE.md have avg session_quality of 4.2 vs 3.1 without" → validates CLAUDE.md investment
- "Sessions with >2 agents have higher scope_creep rate" → suggests agent scoping needs work
- "resume_callback rate is 80% when ai_alignment > 4" → validates the tailoring approach
- "time_to_first_commit correlates with friction_rate" → confirms friction impacts velocity

These correlations surface in `claudewatch suggest` as data-backed recommendations.

---

## 18. Critical Files for Implementation

These files should be read when building claudewatch to understand exact data formats and project patterns:

- `~/.claude/usage-data/session-meta/11c3ec2c-7594-48db-88e9-4c01ce32a012.json` — Richest session-meta example
- `~/.claude/usage-data/facets/debc380f-ea95-4964-9216-f18a62169452.json` — Facet with multiple friction types
- `~/.claude/stats-cache.json` — Stats cache format
- `~/code/shelfctl/internal/app/root.go` — Cobra CLI pattern to follow
- `~/code/shelfctl/.goreleaser.yml` — goreleaser template to replicate
