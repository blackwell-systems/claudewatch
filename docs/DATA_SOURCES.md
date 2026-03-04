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
