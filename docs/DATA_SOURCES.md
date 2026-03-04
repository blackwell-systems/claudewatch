# claudewatch Data Sources

This document describes every file and directory that claudewatch reads from Claude Code's local storage. It is intended for developers working on claudewatch internals.

---

## Table of Contents

1. [Overview of Claude Code's storage layout](#1-overview-of-claude-codes-storage-layout)
2. [Session metadata — `usage-data/session-meta/*.json`](#2-session-metadata)
3. [JSONL session transcripts — `projects/{hash}/{sessionID}.jsonl`](#3-jsonl-session-transcripts)
4. [Facets — `usage-data/facets/*.json`](#4-facets)
5. [History — `history.jsonl`](#5-history)
6. [Stats cache — `stats-cache.json`](#6-stats-cache)
7. [Settings — `settings.json`](#7-settings)
8. [Custom slash commands — `commands/*.md`](#8-custom-slash-commands)
9. [Plugins — `plugins/installed_plugins.json`](#9-plugins)
10. [Todos — `todos/*.json`](#10-todos)
11. [File history — `file-history/{sessionID}/`](#11-file-history)
12. [claudewatch-owned files](#12-claudewatch-owned-files)
13. [The freshness/staleness problem](#13-the-freshnessstaleness-problem)
14. [Data flow diagram](#14-data-flow-diagram)
15. [Real-world examples](#15-real-world-examples)
16. [Session lifecycle timeline](#16-session-lifecycle-timeline)
17. [Performance characteristics](#17-performance-characteristics)
18. [Claude Code internals (the "why" behind the data model)](#18-claude-code-internals-the-why-behind-the-data-model)
19. [Worked example: Full trace of one session](#19-worked-example-full-trace-of-one-session)

---

## 1. Overview of Claude Code's storage layout

All Claude Code data lives under `~/.claude/` (configurable via `claude_home` in `~/.config/claudewatch/config.yaml`; default constant in `internal/config/defaults.go`).

```
~/.claude/
├── history.jsonl                         # prompt history (one entry per user turn)
├── settings.json                         # global settings, hooks, permissions
├── stats-cache.json                      # aggregate usage stats (written by Claude Code)
├── commands/                             # custom slash command .md files
│   └── *.md
├── plugins/
│   └── installed_plugins.json            # installed plugin registry
├── todos/
│   └── {sessionID}-agent-{agentID}.json  # per-session/agent task lists
├── file-history/
│   └── {sessionID}/                      # versioned file snapshots per session
│       └── {hash}@v{N}
├── projects/
│   └── {projectHash}/                    # one dir per project (hash of cwd)
│       └── {sessionID}.jsonl             # full conversation transcript (append-only)
└── usage-data/
    ├── session-meta/
    │   └── {sessionID}.json              # closed-session summary (written at session close)
    └── facets/
        └── {sessionID}.json              # qualitative session analysis
```

The **primary code** for reading all of these is in `internal/claude/`.

---

## 2. Session metadata

**Path:** `~/.claude/usage-data/session-meta/{sessionID}.json`

**Written by:** Claude Code at session close (not during the session).

**Read by:** `internal/claude/session_meta.go` — `ParseAllSessionMeta()` and `ParseSessionMeta()`.

### SessionMeta struct

```go
// internal/claude/types.go
type SessionMeta struct {
    SessionID             string         `json:"session_id"`
    ProjectPath           string         `json:"project_path"`
    StartTime             string         `json:"start_time"`             // RFC3339
    DurationMinutes       int            `json:"duration_minutes"`
    UserMessageCount      int            `json:"user_message_count"`
    AssistantMessageCount int            `json:"assistant_message_count"`
    ToolCounts            map[string]int `json:"tool_counts"`
    Languages             map[string]int `json:"languages"`
    GitCommits            int            `json:"git_commits"`
    GitPushes             int            `json:"git_pushes"`
    InputTokens           int            `json:"input_tokens"`
    OutputTokens          int            `json:"output_tokens"`
    FirstPrompt           string         `json:"first_prompt"`
    UserInterruptions     int            `json:"user_interruptions"`
    UserResponseTimes     []float64      `json:"user_response_times"`
    ToolErrors            int            `json:"tool_errors"`
    ToolErrorCategories   map[string]int `json:"tool_error_categories"`
    UsesTaskAgent         bool           `json:"uses_task_agent"`
    UsesMCP               bool           `json:"uses_mcp"`
    UsesWebSearch         bool           `json:"uses_web_search"`
    UsesWebFetch          bool           `json:"uses_web_fetch"`
    LinesAdded            int            `json:"lines_added"`
    LinesRemoved          int            `json:"lines_removed"`
    FilesModified         int            `json:"files_modified"`
    MessageHours          []int          `json:"message_hours"`
    UserMessageTimestamps []string       `json:"user_message_timestamps"`
}
```

### What claudewatch derives from it

The `SessionMeta` struct feeds nearly every claudewatch metric:

| Fields used | Derived metric |
|---|---|
| `InputTokens`, `OutputTokens` | Cost estimation (via `analyzer.EstimateSessionCost`) |
| `StartTime`, `DurationMinutes` | Session timeline, daily spend |
| `UserMessageCount`, `AssistantMessageCount` | Session size, productivity proxies |
| `ToolErrors`, `ToolErrorCategories` | Per-project avg tool error rate |
| `GitCommits` | Zero-commit rate (`analyzer.AnalyzeCommits`) |
| `ProjectPath` | Project grouping (`filepath.Base(ProjectPath)`) |
| `UsesTaskAgent` | Agent adoption tracking |
| `UserMessageTimestamps` | JSONL overlay (staleness correction) |

### How it is loaded

`ParseAllSessionMeta(claudeHome)` walks the `session-meta/` directory, reads each `.json` file, and applies the JSONL staleness overlay (see [Section 13](#13-the-freshnessstaleness-problem)) before returning results. It skips any file that fails to parse rather than aborting.

---

## 3. JSONL session transcripts

**Path:** `~/.claude/projects/{projectHash}/{sessionID}.jsonl`

**Written by:** Claude Code continuously during a session — one JSON object per line, appended as events occur. The file is held open by the Claude Code process while the session is active.

**Read by:**
- `internal/claude/transcripts.go` — `ParseSessionTranscripts()`, `ParseSingleTranscript()`, `WalkTranscriptEntries()`
- `internal/claude/active.go` — `ParseActiveSession()`
- `internal/claude/active_live.go` — all `ParseLive*` functions

### Directory structure

Each project gets a directory named after a hash of the project's working directory path. Inside, each session gets one `.jsonl` file named by session UUID. Subdirectories under a project hash directory (e.g. `{sessionID}/subagents/`) are skipped by claudewatch's index builder.

### JSONL entry structure

Every line in a transcript file is a JSON object conforming to `TranscriptEntry`:

```go
// internal/claude/transcripts.go
type TranscriptEntry struct {
    Type            string          `json:"type"`
    Timestamp       string          `json:"timestamp"`    // RFC3339Nano
    SessionID       string          `json:"sessionId"`
    Cwd             string          `json:"cwd"`          // on SessionStart progress entry only
    Message         json.RawMessage `json:"message"`      // assistant or user message body
    Data            json.RawMessage `json:"data"`         // progress entry payload
    ParentToolUseID string          `json:"parentToolUseID"`
    Operation       string          `json:"operation"`    // "enqueue"|"dequeue" for queue-operation
    Content         string          `json:"content"`      // queue-operation raw text
}
```

Known `type` values and what claudewatch reads from each:

| `type` | Purpose | What claudewatch extracts |
|---|---|---|
| `"assistant"` | Claude's response turn | Token usage (`usage.input_tokens`, `usage.output_tokens`), `tool_use` blocks (name, ID, input), model info |
| `"user"` | User/tool-result turn | `tool_result` blocks (`is_error`, `tool_use_id`, content), human message timestamps |
| `"progress"` | Subagent progress event | `agentId` → `parentToolUseID` mapping for TaskStop correlation |
| `"queue-operation"` | Background task notification | Real completion timestamp, `total_tokens`, `tool_use_id` via XML tags in `content` |
| `"summary"` | Context compaction event | Counted as a compaction (context pressure metric) |

### Message body structures

Assistant messages embed a nested object in the `message` field:

```go
type AssistantMessage struct {
    Role    string         `json:"role"`
    Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
    Type      string          `json:"type"`       // "text","tool_use","tool_result"
    ID        string          `json:"id"`         // tool_use ID
    Name      string          `json:"name"`       // tool name (e.g. "Bash","Edit","Task")
    Input     json.RawMessage `json:"input"`      // tool input parameters
    ToolUseID string          `json:"tool_use_id"`
    Content   json.RawMessage `json:"content"`
    IsError   bool            `json:"is_error"`
    Text      string          `json:"text"`
}
```

Token usage lives inside the assistant message body:

```go
// internal/claude/active.go (assistantMsgUsage)
struct {
    Usage struct {
        InputTokens  int `json:"input_tokens"`
        OutputTokens int `json:"output_tokens"`
    } `json:"usage"`
}
```

### What claudewatch parses from transcripts

**Agent spans** (`ParseSingleTranscript`): Scans for `Task` and `TaskStop` `tool_use` blocks in assistant entries, and matching `tool_result` blocks in user entries, to reconstruct agent lifecycle records (`AgentSpan`). Background task completion times and token counts are backfilled from `queue-operation/enqueue` entries.

**Live session stats** (`ParseActiveSession`): On an in-progress session, reconstructs a partial `SessionMeta` by counting `user`/`assistant` turns and summing token usage.

**Live tool errors** (`ParseLiveToolErrors`): Maps tool_use IDs to names, then finds `tool_result` blocks with `is_error: true`. Computes per-tool error counts and consecutive error streak.

**Live friction** (`ParseLiveFriction`): Detects three patterns:
- `tool_error` — any `tool_result` with `is_error: true`
- `retry` — same tool name appearing 2+ times in the last 3 `tool_use` entries
- `error_burst` — 3+ errors in any 5 consecutive `tool_result` blocks

**Context pressure** (`ParseLiveContextPressure`): Sums `input_tokens` across all assistant turns; uses the most recent assistant turn's `input_tokens` as a proxy for current context size (it reflects the full context sent in that turn). Counts `summary` entries as compactions.

**Token velocity** (`ParseLiveTokenWindow`): Filters assistant entries to those within a rolling time window, sums tokens, and computes tokens/minute.

**Cost velocity** (`ParseLiveCostVelocity`): Same window filter, computes USD cost using configurable per-million-token pricing.

**Commit-to-attempt ratio** (`ParseLiveCommitAttempts`): Counts `Edit`/`Write` tool uses and `Bash` tool uses whose `input` contains `"git commit"`.

**Active vs idle time** (`ParseLiveActiveTime`): Collects all entry timestamps; any gap `> 5 minutes` between consecutive messages is counted as idle time.

### Active session detection

`FindActiveSessionPath(claudeHome)` locates the JSONL file for the currently running Claude Code process:

1. Enumerate all `*.jsonl` files under `projects/`.
2. Run `lsof -c claude -F n` (3-second timeout, scoped to Claude processes) and cross-reference the results against the enumerated paths.
3. If lsof finds multiple matches, return the most recently modified.
4. Fallback: if lsof fails or finds nothing, return the most recently modified `.jsonl` if its mtime is within 5 minutes.

Symlinks under `~/.claude` are resolved via `filepath.EvalSymlinks` before building the path set, because `lsof` reports resolved paths.

---

## 4. Facets

**Path:** `~/.claude/usage-data/facets/{sessionID}.json`

**Written by:** Claude Code at session close (qualitative analysis of the session).

**Read by:** `internal/claude/facets.go` — `ParseAllFacets()`.

### SessionFacet struct

```go
// internal/claude/types.go
type SessionFacet struct {
    UnderlyingGoal         string         `json:"underlying_goal"`
    GoalCategories         map[string]int `json:"goal_categories"`
    Outcome                string         `json:"outcome"`
    UserSatisfactionCounts map[string]int `json:"user_satisfaction_counts"`
    ClaudeHelpfulness      string         `json:"claude_helpfulness"`
    SessionType            string         `json:"session_type"`
    FrictionCounts         map[string]int `json:"friction_counts"`
    FrictionDetail         string         `json:"friction_detail"`
    PrimarySuccess         string         `json:"primary_success"`
    BriefSummary           string         `json:"brief_summary"`
    SessionID              string         `json:"session_id"`
}
```

### What claudewatch derives from it

- `FrictionCounts` — friction rate per project (fraction of sessions with non-empty friction), top friction type rankings
- `FrictionCounts` — `get_session_friction` MCP tool (per-session friction events)
- Facets are cross-joined with `SessionMeta` for CLAUDE.md effectiveness analysis (`analyzer.AnalyzeEffectiveness`)
- `get_recent_sessions` uses facet friction counts as the `friction_score` field

---

## 5. History

**Path:** `~/.claude/history.jsonl`

**Written by:** Claude Code — one entry per user prompt, appended in real time.

**Read by:** `internal/claude/history.go` — `ParseHistory()`, `LatestSessionID()`.

### HistoryEntry struct

```go
// internal/claude/types.go
type HistoryEntry struct {
    Display        string         `json:"display"`
    PastedContents map[string]any `json:"pastedContents"`
    Timestamp      int64          `json:"timestamp"`     // Unix ms
    Project        string         `json:"project"`
    SessionID      string         `json:"sessionId"`
}
```

`LatestSessionID` is used as a fallback to identify the most recent session when no active session path is found via `lsof`. The scanner uses a 1 MB per-line buffer to handle large pasted content.

---

## 6. Stats cache

**Path:** `~/.claude/stats-cache.json`

**Written by:** Claude Code (periodically recomputed aggregate of all sessions).

**Read by:** `internal/claude/stats.go` — `ParseStatsCache()`.

### StatsCache struct (key fields)

```go
// internal/claude/types.go
type StatsCache struct {
    Version          int                   `json:"version"`
    LastComputedDate string                `json:"lastComputedDate"`
    DailyActivity    []DailyActivity       `json:"dailyActivity"`
    DailyModelTokens []DailyModelTokens    `json:"dailyModelTokens"`
    ModelUsage       map[string]ModelUsage `json:"modelUsage"`
    TotalSessions    int                   `json:"totalSessions"`
    // ...
}

type ModelUsage struct {
    InputTokens              int64   `json:"inputTokens"`
    OutputTokens             int64   `json:"outputTokens"`
    CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`
    CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`
    CostUSD                  float64 `json:"costUSD"`
    ContextWindow            int     `json:"contextWindow"`
    // ...
}
```

### What claudewatch derives from it

`analyzer.ComputeCacheRatio` reads `ModelUsage.CacheReadInputTokens` and `CacheCreationInputTokens` to estimate the fraction of input tokens that are cache hits. This `CacheRatio` is then used by `analyzer.EstimateSessionCost` to adjust raw token counts into a more accurate cost estimate, since cached tokens are billed at a lower rate than fresh input tokens.

---

## 7. Settings

**Path:** `~/.claude/settings.json`

**Written by:** User (manually or via `claude config`).

**Read by:** `internal/claude/settings.go` — `ParseSettings()`.

### GlobalSettings struct

```go
// internal/claude/types.go
type GlobalSettings struct {
    IncludeCoAuthoredBy bool                   `json:"includeCoAuthoredBy"`
    Permissions         Permissions            `json:"permissions"`
    Hooks               map[string][]HookGroup `json:"hooks"`
    EnabledPlugins      map[string]bool        `json:"enabledPlugins"`
    Preferences         map[string]string      `json:"preferences"`
    EffortLevel         string                 `json:"effortLevel"`
}

type HookGroup struct {
    Matcher string `json:"matcher,omitempty"`
    Hooks   []Hook `json:"hooks"`
}

type Hook struct {
    Type    string `json:"type"`
    Command string `json:"command"`
}
```

### What claudewatch derives from it

- Presence of `Hooks` entries contributes to the hook adoption score in project readiness scoring (`internal/scanner/score.go`).
- Hook presence is used by the suggest engine to recommend or validate PostToolUse hook adoption.
- MCP server registration is also documented in settings: `mcpServers.claudewatch` points to the claudewatch binary — though claudewatch reads this file rather than writing it.

---

## 8. Custom slash commands

**Path:** `~/.claude/commands/*.md`

**Written by:** User (custom slash command definitions).

**Read by:** `internal/claude/commands.go` — `ListCommands()`.

### CommandFile struct

```go
// internal/claude/types.go
type CommandFile struct {
    Name    string   // filename without .md
    Path    string   // absolute path
    Content string   // full markdown content
}
```

Used by the suggest engine to check whether recommended commands (e.g. `/saw`, `/release`) are already installed, and to avoid generating redundant suggestions.

---

## 9. Plugins

**Path:** `~/.claude/plugins/installed_plugins.json`

**Written by:** Claude Code plugin manager.

**Read by:** `internal/claude/plugins.go` — `ParsePlugins()`.

### InstalledPlugins struct

```go
// internal/claude/types.go
type InstalledPlugins struct {
    Version int                             `json:"version"`
    Plugins map[string][]PluginInstallation `json:"plugins"`
}

type PluginInstallation struct {
    Scope        string `json:"scope"`
    InstallPath  string `json:"installPath"`
    Version      string `json:"version"`
    InstalledAt  string `json:"installedAt"`
    LastUpdated  string `json:"lastUpdated"`
    GitCommitSha string `json:"gitCommitSha"`
}
```

The parser handles three possible formats (structured with version, plain map, array) defensively, since the schema has varied across Claude Code versions. Contributes to the plugin usage readiness score.

---

## 10. Todos

**Path:** `~/.claude/todos/{sessionID}-agent-{agentID}.json`

**Written by:** Claude Code — updated whenever Claude's internal task list changes.

**Read by:** `internal/claude/todos.go` — `ParseAllTodos()`.

### TodoTask struct

```go
// internal/claude/types.go
type TodoTask struct {
    Content    string `json:"content"`
    Status     string `json:"status"`     // "pending","in_progress","completed"
    ID         string `json:"id,omitempty"`
    ActiveForm string `json:"activeForm,omitempty"`
}
```

Filename convention: `{sessionID}-agent-{agentID}.json`. The `parseTodoFilename` helper splits on the `-agent-` separator. Currently used by the scanning layer to detect active sessions and task completion rates.

---

## 11. File history

**Path:** `~/.claude/file-history/{sessionID}/{hash}@v{N}`

**Written by:** Claude Code — versioned snapshots of files edited during a session.

**Read by:** `internal/claude/filehistory.go` — `ParseAllFileHistory()`.

### FileHistorySession struct

```go
// internal/claude/types.go
type FileHistorySession struct {
    SessionID   string
    UniqueFiles int   // distinct file hashes
    TotalEdits  int   // total version count across all files
    MaxVersion  int   // highest version number seen in this session
    TotalBytes  int64 // cumulative size of all versioned files
}
```

claudewatch reads only directory listings and file metadata (via `f.Info().Size()`); it never reads file content. The `{hash}@v{N}` filename convention is parsed by `parseVersionedFilename` which splits on the last `@v` separator. Used to cross-validate edit counts against `SessionMeta.FilesModified` and as a data point for session productivity scoring.

---

## 12. claudewatch-owned files

These are files claudewatch reads and writes itself (not Claude Code's files).

### SQLite database

**Path:** `~/.config/claudewatch/claudewatch.db`

**Purpose:** Persistent store for scan snapshots, project scores, friction events, suggestions, agent tasks, and custom metrics. Managed by `internal/store/`. claudewatch writes to this on every `claudewatch scan` run. Most MCP tools query directly from Claude Code's files and do not use the database.

### Session tag store

**Path:** `~/.config/claudewatch/tags.json`

**Purpose:** User-managed overrides for session → project name mapping. Format: `{"session_uuid": "project_name"}`. Written atomically (write-to-temp then rename) by `internal/store/tags.go` via the `set_session_project` MCP tool or `claudewatch tag` CLI command. Read on every MCP request that resolves a project name.

### Config file

**Path:** `~/.config/claudewatch/config.yaml`

**Purpose:** claudewatch's own configuration (scan paths, `claude_home` override, scoring weights, friction thresholds). Loaded by `internal/config/config.go` using Viper. Absence of the file is not an error — defaults are used.

---

## 13. The freshness/staleness problem

### Why it exists

`session-meta/{sessionID}.json` is written by Claude Code **at session close**. While a session is active, the file either does not exist yet or reflects only the state at the previous close (for a resumed session). This means:

- `UserMessageCount`, `AssistantMessageCount`, `InputTokens`, `OutputTokens`, and `DurationMinutes` are all frozen at whatever value Claude Code last wrote.
- The corresponding JSONL transcript (`projects/{hash}/{sessionID}.jsonl`) is updated continuously and always reflects current state.

### Detection: mtime comparison

In `ParseAllSessionMeta` (`internal/claude/session_meta.go`):

```go
metaInfo, metaErr := entry.Info()
jsonlInfo, jsonlErr := os.Stat(jsonlPath)
if metaErr == nil && jsonlErr == nil && jsonlInfo.ModTime().After(metaInfo.ModTime()) {
    // JSONL is newer → session is still active → meta is stale
    if live, err := ParseActiveSession(jsonlPath); err == nil && live != nil {
        // overlay only JSONL-derivable fields
        meta.UserMessageCount = live.UserMessageCount
        meta.AssistantMessageCount = live.AssistantMessageCount
        meta.InputTokens = live.InputTokens
        meta.OutputTokens = live.OutputTokens
        meta.UserMessageTimestamps = live.UserMessageTimestamps
        meta.DurationMinutes = int(time.Since(startTime).Minutes())
    }
}
```

A `buildSessionJSONLIndex` pass walks all `projects/` directories first, building a `sessionID → JSONL path` map, so the mtime comparison is O(1) per session.

### Selective overlay

Only fields that claudewatch can independently derive from the JSONL are updated. Fields that Claude Code computes from internal state not exposed in the transcript (`GitCommits`, `Languages`, `LinesAdded`, `LinesRemoved`, `FilesModified`, `ToolCounts`, etc.) are preserved from the JSON file, even though they may lag.

### Live-first for MCP tools

When MCP tools need the most current session data (e.g. `get_session_stats`), they use a separate live-first path:

1. Call `FindActiveSessionPath` (lsof + mtime heuristic).
2. If an active session is found, call `ParseActiveSession` directly and return with `"live": true` — bypassing `ParseAllSessionMeta` entirely.
3. Only if no active session is found does the tool fall through to `ParseAllSessionMeta`.

---

## 14. Data flow diagram

```
Claude Code process
│
│  writes continuously
│
├─► ~/.claude/projects/{hash}/{sessionID}.jsonl     (transcript, append-only)
│
│  writes at session close
│
├─► ~/.claude/usage-data/session-meta/{sessionID}.json
├─► ~/.claude/usage-data/facets/{sessionID}.json
│
│  writes on prompt submission
│
├─► ~/.claude/history.jsonl
│
│  writes periodically
│
└─► ~/.claude/stats-cache.json
    ~/.claude/settings.json                         (written by user)
    ~/.claude/commands/*.md                         (written by user)
    ~/.claude/plugins/installed_plugins.json        (written by plugin manager)
    ~/.claude/todos/{sessionID}-agent-{agentID}.json
    ~/.claude/file-history/{sessionID}/{hash}@vN


claudewatch reads all of the above
│
├─ internal/claude/session_meta.go  ParseAllSessionMeta()
│    ├─ reads session-meta/*.json
│    └─ overlays from JSONL when JSONL mtime > JSON mtime   (staleness fix)
│
├─ internal/claude/active.go        FindActiveSessionPath() + ParseActiveSession()
│    ├─ enumerates projects/**/*.jsonl
│    ├─ runs lsof -c claude to find open files
│    └─ parses JSONL line-by-line for live stats
│
├─ internal/claude/active_live.go   ParseLive*() functions
│    └─ reads same JSONL for tool errors, friction, context, cost velocity
│
├─ internal/claude/transcripts.go   ParseSessionTranscripts()
│    └─ walks all projects/**/*.jsonl for AgentSpan extraction
│
├─ internal/claude/facets.go        ParseAllFacets()
├─ internal/claude/history.go       ParseHistory()
├─ internal/claude/stats.go         ParseStatsCache()
├─ internal/claude/settings.go      ParseSettings()
├─ internal/claude/commands.go      ListCommands()
├─ internal/claude/plugins.go       ParsePlugins()
├─ internal/claude/todos.go         ParseAllTodos()
└─ internal/claude/filehistory.go   ParseAllFileHistory()
│
▼
internal/analyzer/    (compute metrics, friction, costs, effectiveness)
internal/mcp/         (MCP server — exposes results to Claude Code as tools)
internal/store/       (persist scan results to ~/.config/claudewatch/claudewatch.db)
internal/suggest/     (generate ranked improvement suggestions)
```

### Read frequency by MCP tool call

| Triggered by | Files read |
|---|---|
| `get_session_stats` | `lsof` + active JSONL (if live), else `session-meta/*.json` + `stats-cache.json` |
| `get_recent_sessions` | `session-meta/*.json`, `facets/*.json`, `stats-cache.json` |
| `get_project_health` | `lsof` + active JSONL, `session-meta/*.json`, `facets/*.json`, all transcript JSONL |
| `get_live_tool_errors` | `lsof` + active JSONL only |
| `get_live_friction` | `lsof` + active JSONL only |
| `get_context_pressure` | `lsof` + active JSONL only |
| `get_cost_velocity` | `lsof` + active JSONL only |
| `get_agent_performance` | All transcript JSONL files |
| `get_effectiveness` | `session-meta/*.json`, `facets/*.json`, `stats-cache.json`, `CLAUDE.md` in project dirs |
| `get_session_friction` | `facets/*.json` |
| `set_session_project` | `~/.config/claudewatch/tags.json` (read + write) |
| `claudewatch scan` | All of the above, writes to `claudewatch.db` |

---

## 15. Real-world examples

### Example SessionMeta file

`~/.claude/usage-data/session-meta/20260304-101523-a1b2c3d4.json`

```json
{
  "session_id": "20260304-101523-a1b2c3d4",
  "project_path": "/Users/dayna/code/claudewatch",
  "start_time": "2026-03-04T10:15:23Z",
  "duration_minutes": 42,
  "user_message_count": 12,
  "assistant_message_count": 15,
  "tool_counts": {
    "Read": 18,
    "Edit": 8,
    "Bash": 5,
    "Glob": 3,
    "Write": 2
  },
  "languages": {
    "go": 14,
    "markdown": 2
  },
  "git_commits": 2,
  "git_pushes": 1,
  "input_tokens": 45230,
  "output_tokens": 12140,
  "first_prompt": "add metrics export feature",
  "user_interruptions": 1,
  "user_response_times": [12.5, 8.3, 5.1, 15.2],
  "tool_errors": 2,
  "tool_error_categories": {
    "file_not_found": 1,
    "command_failed": 1
  },
  "uses_task_agent": false,
  "uses_mcp": true,
  "uses_web_search": false,
  "uses_web_fetch": false,
  "lines_added": 847,
  "lines_removed": 34,
  "files_modified": 6,
  "message_hours": [10, 10, 10, 10, 10, 11],
  "user_message_timestamps": [
    "2026-03-04T10:15:23Z",
    "2026-03-04T10:18:45Z",
    "2026-03-04T10:25:12Z"
  ]
}
```

### Example JSONL transcript entries

`~/.claude/projects/abc123def456/20260304-101523-a1b2c3d4.jsonl` (selected lines)

**User message:**
```json
{"type":"user","timestamp":"2026-03-04T10:15:23.456Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"text","text":"add metrics export feature"}]}}
```

**Assistant message with tool use:**
```json
{"type":"assistant","timestamp":"2026-03-04T10:15:25.123Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"assistant","content":[{"type":"text","text":"I'll add a metrics export feature..."},{"type":"tool_use","id":"toolu_01ABC123","name":"Read","input":{"file_path":"/Users/dayna/code/claudewatch/internal/analyzer/metrics.go"}}],"usage":{"input_tokens":4521,"output_tokens":342}}}
```

**Tool result:**
```json
{"type":"user","timestamp":"2026-03-04T10:15:26.789Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01ABC123","content":"file content here...","is_error":false}]}}
```

**Queue operation (background agent completion):**
```json
{"type":"queue-operation","operation":"enqueue","timestamp":"2026-03-04T10:42:15.234Z","content":"<task_id>abc123</task_id><agent_name>Scout</agent_name><total_tokens>8521</total_tokens><tool_use_id>toolu_01XYZ789</tool_use_id>","sessionId":"20260304-101523-a1b2c3d4"}
```

**Context compaction:**
```json
{"type":"summary","timestamp":"2026-03-04T10:35:12.456Z","sessionId":"20260304-101523-a1b2c3d4","data":{"compacted_turns":15,"tokens_before":180000,"tokens_after":95000}}
```

### Example SessionFacet file

`~/.claude/usage-data/facets/20260304-101523-a1b2c3d4.json`

```json
{
  "session_id": "20260304-101523-a1b2c3d4",
  "underlying_goal": "Implement metrics export functionality for external observability platforms",
  "goal_categories": {
    "feature_development": 1
  },
  "outcome": "successful",
  "user_satisfaction_counts": {
    "satisfied": 1
  },
  "claude_helpfulness": "very_helpful",
  "session_type": "feature_implementation",
  "friction_counts": {
    "retry:Bash": 1,
    "tool_error": 1
  },
  "friction_detail": "Minor linter errors requiring retry",
  "primary_success": "Complete metrics export feature with tests and documentation",
  "brief_summary": "Added Prometheus metrics export with privacy-safe aggregation"
}
```

### Example StatsCache excerpt

`~/.claude/stats-cache.json` (partial)

```json
{
  "version": 3,
  "lastComputedDate": "2026-03-04",
  "totalSessions": 487,
  "modelUsage": {
    "claude-sonnet-4": {
      "inputTokens": 12450000,
      "outputTokens": 3210000,
      "cacheReadInputTokens": 8500000,
      "cacheCreationInputTokens": 450000,
      "costUSD": 234.56,
      "contextWindow": 200000
    },
    "claude-opus-4": {
      "inputTokens": 2340000,
      "outputTokens": 890000,
      "costUSD": 89.23,
      "contextWindow": 200000
    }
  },
  "dailyActivity": [
    {
      "date": "2026-03-04",
      "sessions": 8,
      "totalDurationMinutes": 342,
      "totalInputTokens": 345000,
      "totalOutputTokens": 89000
    }
  ]
}
```

---

## 16. Session lifecycle timeline

Understanding **when** each file gets written is critical to understanding staleness and why certain operations succeed or fail.

### Active session (T=0 to T=end)

```
T+0.0s    User launches Claude Code, navigates to project directory
          └─ Claude Code checks ~/.claude/settings.json for hooks/permissions

T+0.2s    User types first prompt: "add metrics export feature"
          └─ ~/.claude/history.jsonl ← appended with HistoryEntry
          └─ sessionId generated: 20260304-101523-a1b2c3d4

T+0.5s    Claude Code creates transcript file (if first session in this project)
          └─ ~/.claude/projects/{projectHash}/20260304-101523-a1b2c3d4.jsonl created
          └─ File held open by Claude Code process (append mode)

T+1.0s    First assistant response
          └─ JSONL ← {"type":"assistant",...,"usage":{"input_tokens":4521,...}}
          └─ JSONL is flushed to disk immediately (crash safety)

T+1.5s    Tool execution (Read tool)
          └─ JSONL ← {"type":"assistant",...,"content":[{"type":"tool_use",...}]}
          └─ JSONL ← {"type":"user",...,"content":[{"type":"tool_result",...}]}

T+2.0s    File modification (Edit tool)
          └─ ~/.claude/file-history/20260304-101523-a1b2c3d4/{hash}@v1 created
          └─ Changed file written to file-history (snapshot)

[... 40 minutes of back-and-forth ...]

T+35m     Context approaching limit
          └─ JSONL ← {"type":"summary",...} (compaction event)
          └─ Older turns summarized, full transcript still preserved

T+42m     User types "exit" or closes session
          └─ Claude Code begins session close sequence
```

### Session close (T=end)

```
T+42m00s  Session close triggered
          └─ Claude Code computes final statistics from JSONL

T+42m01s  SessionMeta written
          └─ ~/.claude/usage-data/session-meta/20260304-101523-a1b2c3d4.json created
          └─ Contains: token counts, tool counts, git commits, duration, etc.
          └─ All values are FINAL (frozen at this instant)

T+42m02s  SessionFacet analysis
          └─ Claude Code analyzes full session for qualitative metrics
          └─ ~/.claude/usage-data/facets/20260304-101523-a1b2c3d4.json created
          └─ Contains: goal, outcome, friction, satisfaction

T+42m03s  JSONL file closed
          └─ File descriptor released (no longer held open)
          └─ File mtime = session close time

T+42m04s  StatsCache updated
          └─ ~/.claude/stats-cache.json recomputed
          └─ Aggregates all sessions, updates daily totals
```

### Key timing implications

**Why session-meta can be stale:**
- SessionMeta written at T+42m (session close)
- JSONL continuously appended until T+42m
- If session is RESUMED later, JSONL continues appending but SessionMeta stays frozen
- claudewatch detects staleness via `JSONL mtime > SessionMeta mtime`

**Why lsof is necessary for active session detection:**
- During active session (T+0 to T+42m), JSONL file descriptor is OPEN
- After session close (T+42m+), file descriptor is CLOSED
- `lsof -c claude` shows open file descriptors
- mtime alone cannot distinguish "active now" from "closed 2 minutes ago"

**Why facets may have fewer sessions than session-meta:**
- Facet analysis is compute-intensive
- If Claude Code crashes during facet writing (T+42m02s), facet file is not created
- SessionMeta may exist (written at T+42m01s) but facet does not
- This is why `ParseAllFacets` returns fewer entries than `ParseAllSessionMeta`

**Why stats-cache is eventually consistent:**
- Updated at T+42m04s (AFTER session-meta and facets)
- If you query stats-cache immediately after a session, it may not include that session yet
- Next session close will update it
- claudewatch MCP tools bypass stats-cache and read session-meta directly for accuracy

---

## 17. Performance characteristics

claudewatch's performance depends heavily on **which files it reads** and **how many sessions exist**.

### Fast operations (< 100ms)

These read only lightweight index files:

**`get_session_stats` (active session):**
- `lsof -c claude -F n` (3-second timeout, but typically 10-50ms)
- Parse single JSONL file (streaming, stops at last `assistant` entry with token usage)
- **Cost**: O(active_session_size), typically 500-2000 lines

**`get_recent_sessions` (N=5):**
- Read `session-meta/*.json` (file enumeration, stat, read)
- Read `facets/*.json` (same)
- Read `stats-cache.json` (single file)
- **Cost**: O(N), where N is number of sessions requested (default 5)

**`set_session_project`:**
- Read `tags.json` (single small file, <10KB)
- Write `tags.json` (atomic write-to-temp + rename)
- **Cost**: O(1)

### Medium operations (100ms - 1s)

These scan all session metadata but not transcripts:

**`get_project_health`:**
- Walk `session-meta/` directory (enumerate all .json files)
- Parse each SessionMeta (JSON decode)
- Walk `facets/` directory (same)
- **Cost**: O(total_sessions), where total_sessions is typically 100-1000
- **Bottleneck**: Filesystem directory listing + JSON parsing

**`get_effectiveness`:**
- Same as `get_project_health`
- Plus: read multiple `CLAUDE.md` files from project directories
- **Cost**: O(total_sessions) + O(projects)

**`get_cost_velocity` / `get_context_pressure` (live session):**
- Parse single active JSONL (full file, not streaming)
- Extract all `assistant` entries with token counts
- Compute rolling window stats
- **Cost**: O(active_session_size), typically 2000-5000 lines

### Slow operations (1s - 10s)

These parse full JSONL transcripts for all sessions:

**`get_agent_performance`:**
- Walk `projects/` directory recursively (all subdirectories)
- Parse EVERY `.jsonl` file fully (not streaming)
- Extract `Task` and `TaskStop` tool uses
- Reconstruct agent spans from tool_use/tool_result pairs
- **Cost**: O(total_sessions × avg_transcript_size)
- **Bottleneck**: JSONL parsing (each line is JSON decoded)

**`get_saw_sessions` + `get_saw_wave_breakdown`:**
- Same as `get_agent_performance`
- Additional filtering for SAW-tagged agents (description contains `[SAW:wave`)
- **Cost**: O(total_sessions × avg_transcript_size)

**`get_project_health` (with agent metrics):**
- Combines `get_project_health` (medium) + `get_agent_performance` (slow)
- **Cost**: O(total_sessions) + O(total_sessions × avg_transcript_size)

### Very slow operations (10s+)

**`claudewatch scan` (full index rebuild):**
- All of the above operations combined
- Plus: `ParseAllFileHistory` (walks `file-history/{sessionID}/` for all sessions)
- Plus: friction detection across all sessions
- Plus: suggestion generation
- Plus: SQLite writes (insert/update for every session)
- **Cost**: O(total_sessions × avg_transcript_size) + O(total_file_versions)
- **Typical duration**: 5-30 seconds for 500 sessions

### Performance optimization strategies

**1. mtime caching**

claudewatch uses `buildSessionJSONLIndex` to build an in-memory map of `sessionID → (JSONL path, mtime)` once per operation. This avoids repeated filesystem traversals:

```go
// Build index once
index := buildSessionJSONLIndex(claudeHome)

// Check staleness for each session (O(1) lookup)
for each session in session-meta/ {
    jsonlPath, jsonlMtime := index[sessionID]
    if jsonlMtime.After(metaMtime) {
        // stale, overlay from JSONL
    }
}
```

**2. Streaming JSONL parsing**

Active session parsing stops early:

```go
// ParseActiveSession reads line-by-line until it finds enough data
for scanner.Scan() {
    entry := parseJSONLEntry(scanner.Bytes())
    if entry.Type == "assistant" {
        // Extract tokens, increment turn count
        // Don't need to parse the entire file
    }
}
```

For full transcript parsing (`ParseSingleTranscript`), every line must be parsed because agent spans can appear anywhere.

**3. Parallel session parsing**

`ParseAllSessionMeta` could be parallelized (not currently implemented):

```go
// NOT implemented, but would be:
for each metaFile in session-meta/ {
    go func(path string) {
        meta := parseSessionMeta(path)
        results <- meta
    }(metaFile)
}
```

Currently serial because filesystem I/O dominates CPU time.

**4. Database caching**

`claudewatch scan` writes to SQLite (`~/.config/claudewatch/claudewatch.db`). Most MCP tools bypass the database and read files directly because:
- Live data accuracy matters more than speed
- Database can be stale if user hasn't run `scan` recently
- Parsing 500 sessions takes 1-2 seconds, which is acceptable for interactive use

### When to use the database

The database is useful for:
- Queries over historical data (last 30 days, trend analysis)
- Joins across multiple tables (sessions × friction events × suggestions)
- Pre-aggregated metrics that are expensive to compute

MCP tools that DO use the database:
- `get_suggestions` (reads pre-computed suggestion rankings)
- `get_task_history` / `get_blockers` (reads memory extraction results)

---

## 18. Claude Code internals (the "why" behind the data model)

### Why JSONL instead of JSON?

**Streaming writes + crash safety:**

Claude Code writes transcript entries one line at a time as they occur. If the process crashes mid-session:

```
Line 1: {"type":"user",...}
Line 2: {"type":"assistant",...}
Line 3: {"type":"user",...}
[CRASH - file closed unexpectedly]
```

The file is still valid JSONL — you can parse lines 1-3. With a single JSON object, a crash mid-write would leave an invalid file:

```
{"entries":[
  {"type":"user",...},
  {"type":"assistant",...},
  {"type":"user",...
[CRASH - file truncated, not valid JSON]
```

**Append-only performance:**

Appending a line to a file is O(1). Rewriting an entire JSON array is O(file_size).

### Why session-meta is written at session close

**Requires full context:**

Many SessionMeta fields cannot be computed until the session ends:

- `DurationMinutes` — needs start and end timestamps
- `GitCommits` — Claude Code watches for `git commit` tool uses throughout the session
- `ToolCounts` — summed across all turns
- `UserInterruptions` — detected when user sends a message while assistant is "thinking"

**Trade-off: freshness vs completeness**

Claude Code chose **completeness** (write accurate data once at end) over **freshness** (write partial data continuously). This is why claudewatch implements the staleness overlay.

### Why facets are separate from session-meta

**Different write timing:**

- SessionMeta: fast aggregation (token counts, tool counts) — written in <50ms
- Facets: LLM-powered analysis (goal understanding, satisfaction inference) — takes 1-5 seconds

If they were in the same file, session close would be blocked waiting for facet analysis. Separate files allow:

1. Write SessionMeta immediately (fast)
2. Write Facets asynchronously (slow)
3. If facet analysis fails, SessionMeta still exists

**Different data types:**

- SessionMeta: structured counts (integers, maps)
- Facets: qualitative analysis (strings, enums)

They serve different purposes and have different consumers.

### Why stats-cache exists

**Expensive recomputation avoidance:**

Computing aggregate statistics across all sessions requires:
- Parsing every session-meta file
- Summing token counts, computing cost
- Grouping by date, model, project

This takes 1-5 seconds for 500 sessions. Without a cache, every `claude stats` invocation would be slow.

**Trade-off: speed vs accuracy**

Stats-cache is updated at session close. Between sessions, it may be slightly stale. This is acceptable for overview stats but not for precise queries (which bypass the cache).

### Why file-history exists

**Undo capability:**

Claude Code needs to support "undo" for Edit operations. Versioned snapshots allow:

```
user: "edit login.ts"
claude: [writes edit]
user: "undo that"
claude: [reads {hash}@v1, restores previous version]
```

**Audit trail:**

For debugging "what did Claude change?" questions, file-history shows exactly what was edited and when.

**Storage cost:**

File-history can grow large (100s of MB for long-running projects). Claude Code periodically garbage-collects old versions.

### Why lsof is needed

**Distinguishing active from recent:**

Filesystem mtime tells you "when was this file last modified?" but not "is it currently open?"

```
Session A: closed 30 seconds ago (mtime = now-30s)
Session B: active now, last write 5 minutes ago (mtime = now-5m)
```

If you sort by mtime, Session A looks more recent than Session B, but Session B is actually active.

`lsof -c claude` tells you which files the `claude` process has open RIGHT NOW. This is the only reliable way to detect active sessions.

**Fallback for when lsof fails:**

If lsof times out or is unavailable (some systems restrict it), claudewatch falls back to "most recent mtime < 5 minutes ago" heuristic. This is less reliable but better than nothing.

### Why history.jsonl exists separately from transcripts

**Cross-session prompt history:**

`history.jsonl` accumulates prompts across ALL sessions. It enables:

- Arrow-up to recall previous prompts (across sessions)
- Search through command history
- Analytics: "what are users most commonly asking for?"

If prompts were only in per-session transcripts, you'd need to scan 100s of files to find "what did I ask 3 days ago?"

---

## 19. Worked example: Full trace of one session

Let's follow a complete session from start to finish, showing exactly what gets written where and when.

### T+0s: Session start

**User action:** Opens Claude Code, navigates to `/Users/dayna/code/claudewatch`, types "add prometheus metrics export"

**Files written:**

`~/.claude/history.jsonl` (appended):
```json
{"display":"add prometheus metrics export","pastedContents":{},"timestamp":1709550923000,"project":"claudewatch","sessionId":"20260304-101523-a1b2c3d4"}
```

**Files created:**

`~/.claude/projects/a1b2c3d4ef5678/20260304-101523-a1b2c3d4.jsonl`:
```json
{"type":"user","timestamp":"2026-03-04T10:15:23.456Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"text","text":"add prometheus metrics export"}]}}
```

### T+2s: First assistant response

**Claude Code action:** Generates response with tool uses

**JSONL (appended 1 line):**
```json
{"type":"assistant","timestamp":"2026-03-04T10:15:25.123Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"assistant","content":[{"type":"text","text":"I'll add a Prometheus metrics export feature. Let me start by reading the existing analyzer code."},{"type":"tool_use","id":"toolu_01ABC","name":"Read","input":{"file_path":"/Users/dayna/code/claudewatch/internal/analyzer/metrics.go"}}],"usage":{"input_tokens":4521,"output_tokens":182}}}
```

### T+2.1s: Tool result

**Claude Code action:** Executes Read tool, returns result

**JSONL (appended 1 line):**
```json
{"type":"user","timestamp":"2026-03-04T10:15:25.234Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01ABC","content":"[file contents: 450 lines of Go code]","is_error":false}]}}
```

### T+5s: File edit

**Claude Code action:** Assistant uses Edit tool to modify a file

**JSONL (appended 2 lines):**
```json
{"type":"assistant","timestamp":"2026-03-04T10:15:28.567Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01DEF","name":"Edit","input":{"file_path":"/Users/dayna/code/claudewatch/internal/export/exporter.go","old_string":"...","new_string":"..."}}],"usage":{"input_tokens":5234,"output_tokens":234}}}
{"type":"user","timestamp":"2026-03-04T10:15:28.890Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01DEF","content":"File updated successfully","is_error":false}]}}
```

**File-history (created):**

`~/.claude/file-history/20260304-101523-a1b2c3d4/abc123def456@v1`:
```
[snapshot of exporter.go before the edit]
```

### T+10s: User interrupts with new message

**User action:** Types "also add documentation" while Claude is thinking

**JSONL (appended 1 line):**
```json
{"type":"user","timestamp":"2026-03-04T10:15:33.123Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"text","text":"also add documentation"}]}}
```

**SessionMeta will record:** `UserInterruptions: 1`

### T+35m: Context compaction

**Claude Code action:** Context window approaching limit, triggers summarization

**JSONL (appended 1 line):**
```json
{"type":"summary","timestamp":"2026-03-04T10:50:15.456Z","sessionId":"20260304-101523-a1b2c3d4","data":{"compacted_turns":18,"tokens_before":185000,"tokens_after":92000}}
```

### T+40m: Git commit

**Claude Code action:** Assistant runs `git commit` command

**JSONL (appended 2 lines):**
```json
{"type":"assistant","timestamp":"2026-03-04T10:55:12.345Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01GHI","name":"Bash","input":{"command":"git add internal/export/ && git commit -m \"feat(export): add Prometheus exporter\""}}],"usage":{"input_tokens":6234,"output_tokens":145}}}
{"type":"user","timestamp":"2026-03-04T10:55:13.567Z","sessionId":"20260304-101523-a1b2c3d4","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01GHI","content":"[main abc123] feat(export): add Prometheus exporter\n 3 files changed, 450 insertions(+)","is_error":false}]}}
```

**SessionMeta will record:** `GitCommits: 1`, `GitPushes: 0`

### T+42m: Session close

**User action:** Types "exit" or closes Claude Code

**Claude Code action:** Session close sequence begins

#### T+42m00s: Compute final statistics

Claude Code scans the JSONL file and computes:
- Total turns: 27 user, 32 assistant
- Token usage: 185,230 input, 45,120 output
- Tool counts: Read=18, Edit=8, Write=2, Bash=5, Glob=3
- Languages: go=22, markdown=3
- Git commits: 1 (found `git commit` in Bash tool input)
- Tool errors: 2 (found `is_error: true` in tool_result entries)
- Duration: 42 minutes (last timestamp - first timestamp)

#### T+42m01s: Write SessionMeta

**File created:**

`~/.claude/usage-data/session-meta/20260304-101523-a1b2c3d4.json`:
```json
{
  "session_id": "20260304-101523-a1b2c3d4",
  "project_path": "/Users/dayna/code/claudewatch",
  "start_time": "2026-03-04T10:15:23Z",
  "duration_minutes": 42,
  "user_message_count": 27,
  "assistant_message_count": 32,
  "tool_counts": {"Read": 18, "Edit": 8, "Write": 2, "Bash": 5, "Glob": 3},
  "languages": {"go": 22, "markdown": 3},
  "git_commits": 1,
  "git_pushes": 0,
  "input_tokens": 185230,
  "output_tokens": 45120,
  "first_prompt": "add prometheus metrics export",
  "user_interruptions": 1,
  "user_response_times": [12.5, 8.3, 5.1, 15.2],
  "tool_errors": 2,
  "tool_error_categories": {"file_not_found": 1, "command_failed": 1},
  "uses_task_agent": false,
  "uses_mcp": true,
  "uses_web_search": false,
  "uses_web_fetch": false,
  "lines_added": 847,
  "lines_removed": 34,
  "files_modified": 6,
  "message_hours": [10, 10, 10, 10, 10, 10, 10, 10, 11],
  "user_message_timestamps": ["2026-03-04T10:15:23Z", "2026-03-04T10:18:45Z", ...]
}
```

#### T+42m02s: Facet analysis

Claude Code sends the full transcript to an LLM with a prompt: "Analyze this session and extract: goal, outcome, friction, satisfaction"

**File created:**

`~/.claude/usage-data/facets/20260304-101523-a1b2c3d4.json`:
```json
{
  "session_id": "20260304-101523-a1b2c3d4",
  "underlying_goal": "Implement Prometheus metrics export functionality",
  "goal_categories": {"feature_development": 1},
  "outcome": "successful",
  "user_satisfaction_counts": {"satisfied": 1},
  "claude_helpfulness": "very_helpful",
  "session_type": "feature_implementation",
  "friction_counts": {"retry:Bash": 1, "tool_error": 1},
  "friction_detail": "Minor file not found error, one command retry",
  "primary_success": "Complete Prometheus export implementation with tests",
  "brief_summary": "Added metrics export feature with Prometheus text format support"
}
```

#### T+42m03s: JSONL file closed

Claude Code releases the file descriptor for the JSONL file. It's no longer "open" from `lsof`'s perspective.

#### T+42m04s: Update stats-cache

Claude Code recomputes aggregate statistics across all sessions:

**File updated:**

`~/.claude/stats-cache.json` (dailyActivity array gets new entry):
```json
{
  "dailyActivity": [
    {
      "date": "2026-03-04",
      "sessions": 8,
      "totalDurationMinutes": 384,
      "totalInputTokens": 1523450,
      "totalOutputTokens": 423120
    }
  ],
  "modelUsage": {
    "claude-sonnet-4": {
      "inputTokens": 12635230,
      "outputTokens": 3255120,
      "costUSD": 237.84
    }
  }
}
```

### What claudewatch sees

**Immediately during session (T+0 to T+42m):**

If you run `claudewatch mcp get_session_stats`:
1. `FindActiveSessionPath` runs `lsof -c claude`, finds the open JSONL file
2. `ParseActiveSession` reads the JSONL, computes live stats:
   - Tokens: 185230 input, 45120 output (from `usage` fields in assistant messages)
   - Turn counts: 27 user, 32 assistant
   - **Live**: true (indicates data is from active session, not from session-meta)

**After session close (T+42m+):**

If you run `claudewatch mcp get_session_stats`:
1. `FindActiveSessionPath` runs `lsof -c claude`, finds NO open JSONL file
2. Falls back to `ParseAllSessionMeta`
3. Reads `session-meta/20260304-101523-a1b2c3d4.json`
4. Returns the same stats, but **Live**: false

**If session is resumed later:**

User reopens Claude Code in the same project tomorrow, Claude Code finds the existing sessionId and continues:

1. JSONL file is reopened (file descriptor open again)
2. New entries appended (T+24h00s: `{"type":"user",...}`)
3. SessionMeta still shows data from T+42m (stale!)
4. claudewatch detects: `JSONL mtime > SessionMeta mtime`
5. Overlays live data from JSONL (current tokens, turn counts)

---
