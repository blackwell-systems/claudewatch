# IMPL: AI Ops Platform — Tier 1 Features

**Features:**
1. Per-turn cost attribution — break token spend down to the tool-call level
2. Session replay — structured timeline of turns, tools, cost, and friction
3. CLAUDE.md A/B testing — compare two CLAUDE.md variants over a time window

**Repo:** `~/code/claudewatch`

---

## Suitability Assessment

**Verdict: SUITABLE**

### Gate answers

1. **File decomposition.** Yes. The three features decompose into four agents with
   disjoint file ownership. Attribution and replay share only the transcript reading
   path from `internal/claude/`, which is read-only (no modifications); both consume
   `WalkTranscriptEntries`/`ParseSingleTranscript` as a stable interface. The experiment
   (A/B test) feature is entirely self-contained in `internal/store/experiment.go`,
   `internal/analyzer/experiment.go`, and `internal/app/experiment.go`. The one shared
   file that requires sequential ordering is `internal/store/migrations.go`, which needs
   a v3 migration block added by the experiment agent; this is treated as an
   orchestrator-owned post-merge step.

2. **Investigation-first items.** None. All three features operate on transcript data
   and session metadata whose structure is already well understood from existing code
   (`ParseActiveSession`, `WalkTranscriptEntries`, `assistantMsgUsage`, `SessionFacet`,
   `SessionTagStore`). No root-cause investigation is needed before implementation.

3. **Interface discoverability.** Yes. All cross-agent contracts are fully specifiable
   now. The attribution and replay agents read JSONL entries directly via existing
   `claude.WalkTranscriptEntries`; their output types are defined below. The experiment
   agent depends only on SQLite (schema is designed here) and on `store.SessionTagStore`
   which is already stable.

4. **Pre-implementation status check.**

   | Item | Status | Evidence |
   |------|--------|----------|
   | Per-turn attribution store layer (`store/attribution.go`) | TO-DO | No `TurnAttribution` type or `ComputeAttribution` function anywhere |
   | Per-turn attribution CLI (`app/attribute.go`) | TO-DO | No `claudewatch attribute` subcommand |
   | Per-turn attribution MCP tool (`mcp/attribution_tools.go`) | TO-DO | `get_cost_attribution` not in `mcp/tools.go` |
   | Session replay store layer (`store/replay.go`) | TO-DO | No `SessionReplay` or `BuildReplay` anywhere |
   | Session replay CLI (`app/replay.go`) | TO-DO | No `claudewatch replay` subcommand |
   | Experiment store layer (`store/experiment.go`) | TO-DO | No experiments table; schema at v2 |
   | Experiment analyzer (`analyzer/experiment.go`) | TO-DO | No `AnalyzeExperiment` anywhere |
   | Experiment CLI (`app/experiment.go`) | TO-DO | No `claudewatch experiment` subcommand |
   | Schema migration v3 (`store/migrations.go`) | TO-DO | `currentSchemaVersion = 2` |

5. **Parallelization value check.**
   ```
   Scout phase:                                         ~30 min
   Wave 1 — Agent A (store: attribution + replay)      ~30 min
             Agent B (store: experiment + migration)    ~35 min
             [wall-clock = 35 min]
   Wave 2 — Agent C (app CLI: attribute + replay)      ~35 min  [depends on A]
             Agent D (app CLI + MCP: experiment)        ~30 min  [depends on B]
             Agent E (MCP: attribution tool)            ~20 min  [depends on A]
             [wall-clock = 35 min]
   Orchestrator merge + verification:                  ~10 min
   Total SAW time:                                     ~110 min

   Sequential baseline: 30+30+35+35+30+20+30 overhead = ~210 min
   Time savings: ~100 min (~48%)
   Recommendation: Clear time win. Proceed.
   ```

---

## Known Issues and Design Notes

1. **Token counts available per assistant turn, not per tool call.** The JSONL
   transcript has `usage.input_tokens` and `usage.output_tokens` on each `assistant`
   entry (this is what `assistantMsgUsage` extracts in `internal/claude/active.go`).
   There is no per-tool-call token breakdown in the transcript format. Attribution must
   therefore work at the **turn level**: each assistant turn (one JSONL line of type
   `"assistant"`) can carry multiple tool_use blocks. The attribution groups by tool name
   across all turns, summing turn-level tokens. The column header in the output is "Tool
   Type | Calls | Input Tokens | Output Tokens | Est. Cost" as specified — where "Calls"
   is the count of tool_use blocks of that type found in assistant turns.

2. **Model name per turn.** The assistant message JSON carries a `model` field at the
   top level (`{"role":"assistant","content":[...],"model":"claude-sonnet-4-5",...}`).
   Attribution should extract this to allow per-model cost estimation. Define an extended
   internal struct `assistantMsgFull` in `store/attribution.go` for local parsing (do not
   modify `internal/claude/active.go`).

3. **Schema migration v3.** The experiments table must be added in `migrateV3()` in
   `internal/store/migrations.go`. The orchestrator applies this after all agents
   complete. The constant `currentSchemaVersion` must be bumped from 2 to 3.

4. **`min` helper collision.** `internal/app/doctor.go` defines `func min(a, b int) int`.
   This is in package `app`. New files in that package must not redefine `min`. Go 1.21+
   built-in `min` is available in `go 1.26.0` (see `go.mod`); use the built-in or the
   existing package-level one.

5. **Replay is CLI-only; no MCP tool.** The feature spec explicitly excludes
   `internal/mcp/replay_tools.go`. Do not create it.

6. **Mann-Whitney U for A/B comparison.** The feature spec mentions Mann-Whitney U or
   simple mean comparison for the experiment analyzer. Implement simple mean comparison
   plus a basic t-statistic for the winner determination. Mann-Whitney U requires sorting
   and rank assignment — possible in pure Go but adds complexity. Use mean comparison with
   a confidence note. If sample sizes are small (< 5 per variant), emit `"inconclusive"`.

7. **SessionTagStore as variant assignment store.** The `store.SessionTagStore` stores
   `session_id → project_name` in a flat JSON file. The experiment feature stores
   `session_id → variant` assignments in SQLite (separate from the tag store). These are
   independent.

---

## Dependency Graph

```
internal/claude/transcripts.go   (READ ONLY — WalkTranscriptEntries, ParseSingleTranscript,
                                   TranscriptEntry, AssistantMessage, ContentBlock)
         │
         ├──► internal/store/attribution.go   [Agent A]
         │         TurnAttribution, ComputeAttribution
         │
         ├──► internal/store/replay.go         [Agent A]
         │         SessionReplay, ReplayTurn, BuildReplay
         │
         │    internal/store/migrations.go     [Orchestrator post-merge]
         │         migrateV3 (experiments table)
         │         currentSchemaVersion = 3
         │
         └──► internal/store/experiment.go     [Agent B]
                   Experiment, ExperimentResults, CreateExperiment,
                   GetActiveExperiment, RecordSessionVariant, GetExperimentResults

internal/analyzer/experiment.go  [Agent B]
    AnalyzeExperiment, ExperimentReport
    depends on: store.Experiment, claude.SessionMeta, claude.SessionFacet,
                analyzer.EstimateSessionCost (already exists in analyzer/outcomes.go)

internal/app/attribute.go        [Agent C]  depends on store/attribution.go
internal/app/replay.go           [Agent C]  depends on store/replay.go
internal/app/experiment.go       [Agent D]  depends on store/experiment.go, analyzer/experiment.go

internal/mcp/attribution_tools.go [Agent E]  depends on store/attribution.go
```

---

## Interface Contracts

### Store: Attribution

```go
// File: internal/store/attribution.go
package store

// TurnAttribution holds aggregated token and cost data for one tool type.
type TurnAttribution struct {
    ToolType     string  `json:"tool_type"`    // e.g. "Bash", "Read", "Edit", "Task", "text"
    Calls        int     `json:"calls"`
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    EstCostUSD   float64 `json:"est_cost_usd"`
}

// ComputeAttribution reads the JSONL transcript for sessionID under claudeHome/projects/
// and groups token usage by tool type across all assistant turns.
// Returns rows sorted by EstCostUSD descending.
// If sessionID is empty, uses the most recently modified .jsonl file.
func ComputeAttribution(sessionID, claudeHome string, pricing analyzer.ModelPricing) ([]TurnAttribution, error)
```

### Store: Replay

```go
// File: internal/store/replay.go
package store

// ReplayTurn represents one turn in a session timeline.
type ReplayTurn struct {
    Turn        int     `json:"turn"`
    Role        string  `json:"role"`       // "user" | "assistant"
    ToolName    string  `json:"tool_name"`  // first tool_use name, or "" for text turns
    InputTokens int     `json:"input_tokens"`
    OutputTokens int    `json:"output_tokens"`
    EstCostUSD  float64 `json:"est_cost_usd"`
    Friction    bool    `json:"friction"`   // true if role="user" and contains tool_result with is_error=true
    Timestamp   string  `json:"timestamp"`  // RFC3339 or empty
}

// SessionReplay is the ordered timeline for one session.
type SessionReplay struct {
    SessionID    string       `json:"session_id"`
    TotalTurns   int          `json:"total_turns"`
    TotalCostUSD float64      `json:"total_cost_usd"`
    FrictionCount int         `json:"friction_count"`
    Turns        []ReplayTurn `json:"turns"`
}

// BuildReplay reads the JSONL file for sessionID and returns a SessionReplay.
// claudeHome is the Claude home directory; the function locates the JSONL via
// internal path logic (same as WalkTranscriptEntries).
// from and to are 1-indexed inclusive; 0 means no bound.
func BuildReplay(sessionID, claudeHome string, from, to int, pricing analyzer.ModelPricing) (SessionReplay, error)
```

### Store: Experiment

```go
// File: internal/store/experiment.go
package store

import "time"

// Experiment represents an A/B test comparing two CLAUDE.md variants.
type Experiment struct {
    ID        int64     `json:"id"`
    Project   string    `json:"project"`
    StartedAt time.Time `json:"started_at"`
    StoppedAt *time.Time `json:"stopped_at,omitempty"`
    Active    bool      `json:"active"`
    Note      string    `json:"note,omitempty"`
}

// ExperimentSession links a session to a variant assignment.
type ExperimentSession struct {
    ExperimentID int64  `json:"experiment_id"`
    SessionID    string `json:"session_id"`
    Variant      string `json:"variant"` // "a" or "b"
}

// CreateExperiment inserts a new experiment record. Returns the new experiment's ID.
func (db *DB) CreateExperiment(project, note string) (int64, error)

// GetActiveExperiment returns the active experiment for project, or nil if none.
func (db *DB) GetActiveExperiment(project string) (*Experiment, error)

// StopExperiment marks the experiment with id as stopped.
func (db *DB) StopExperiment(id int64) error

// RecordSessionVariant records which variant a session belongs to.
func (db *DB) RecordSessionVariant(experimentID int64, sessionID, variant string) error

// GetExperimentSessions returns all session-variant assignments for an experiment.
func (db *DB) GetExperimentSessions(experimentID int64) ([]ExperimentSession, error)
```

### Analyzer: Experiment

```go
// File: internal/analyzer/experiment.go
package analyzer

import (
    "github.com/blackwell-systems/claudewatch/internal/claude"
    "github.com/blackwell-systems/claudewatch/internal/store"
)

// VariantStats holds per-variant outcome metrics.
type VariantStats struct {
    Variant      string  `json:"variant"`
    SessionCount int     `json:"session_count"`
    AvgCostUSD   float64 `json:"avg_cost_usd"`
    AvgFriction  float64 `json:"avg_friction"`
    AvgCommits   float64 `json:"avg_commits"`
}

// ExperimentReport is the output of an A/B experiment analysis.
type ExperimentReport struct {
    ExperimentID int64        `json:"experiment_id"`
    Project      string       `json:"project"`
    A            VariantStats `json:"variant_a"`
    B            VariantStats `json:"variant_b"`
    Winner       string       `json:"winner"` // "a", "b", or "inconclusive"
    Confidence   string       `json:"confidence"` // "high", "low", "inconclusive"
    Summary      string       `json:"summary"`
}

// AnalyzeExperiment computes per-variant metrics and determines a winner.
// sessions and facets cover all sessions in the experiment window.
// assignments maps sessionID → variant ("a"|"b").
func AnalyzeExperiment(
    exp store.Experiment,
    sessions []claude.SessionMeta,
    facets []claude.SessionFacet,
    assignments map[string]string,
    pricing ModelPricing,
    ratio CacheRatio,
) ExperimentReport
```

### App: attribute subcommand

```go
// File: internal/app/attribute.go
// Command: claudewatch attribute [--session <id>] [--project <name>] [--json]
// Flags:
//   --session string   Session ID to analyze (default: most recent)
//   --project string   Filter to sessions for this project (not used in single-session mode)
// Behavior:
//   - Opens DB, calls store.ComputeAttribution(sessionID, cfg.ClaudeHome, pricing)
//   - Renders table: Tool Type | Calls | Input Tokens | Output Tokens | Est. Cost
//   - --json emits JSON array of TurnAttribution
```

### App: replay subcommand

```go
// File: internal/app/replay.go
// Command: claudewatch replay <session-id> [--from <turn>] [--to <turn>] [--json]
// Flags:
//   --from int   First turn number to show (default 0 = all)
//   --to   int   Last turn number to show (default 0 = all)
// Behavior:
//   - Calls store.BuildReplay(sessionID, cfg.ClaudeHome, from, to, pricing)
//   - Renders table: Turn | Role | Tool | In Tokens | Out Tokens | Cost | Friction
//   - --json emits SessionReplay as JSON
```

### App: experiment subcommand

```go
// File: internal/app/experiment.go
// Command: claudewatch experiment <start|stop|tag|report> [flags]
//
// start  --project <name> [--note <text>]
//        Creates a new experiment for the project.
//
// stop   --project <name>
//        Stops the active experiment for the project.
//
// tag    --project <name> --session <id> --variant <a|b>
//        Records a session's variant assignment.
//        If --session is omitted, tags the most recent session.
//
// report --project <name> [--json]
//        Runs AnalyzeExperiment on the active (or most recent) experiment.
//        Renders a two-column comparison table and a winner line.
```

### MCP: get_cost_attribution

```go
// File: internal/mcp/attribution_tools.go
// Tool name: get_cost_attribution
// Input schema:
//   { "session_id": string (optional), "project": string (optional) }
// If session_id is omitted, uses most recent session.
// Returns: []TurnAttribution as JSON
```

---

## File Ownership Table

| File | Owner | Creates/Modifies | Depends On |
|------|-------|-----------------|------------|
| `internal/store/attribution.go` | Agent A | Creates | `internal/claude/transcripts.go` (read), `internal/analyzer/cost.go` (ModelPricing type) |
| `internal/store/replay.go` | Agent A | Creates | `internal/claude/transcripts.go` (read), `internal/analyzer/cost.go` (ModelPricing type) |
| `internal/store/experiment.go` | Agent B | Creates | `internal/store/db.go` (DB struct) |
| `internal/analyzer/experiment.go` | Agent B | Creates | `internal/claude/types.go`, `internal/store/experiment.go`, `internal/analyzer/outcomes.go` (EstimateSessionCost) |
| `internal/app/attribute.go` | Agent C | Creates | `internal/store/attribution.go`, `internal/app/root.go` (flagJSON, flagNoColor, flagConfig) |
| `internal/app/replay.go` | Agent C | Creates | `internal/store/replay.go`, `internal/app/root.go` |
| `internal/app/experiment.go` | Agent D | Creates | `internal/store/experiment.go`, `internal/analyzer/experiment.go`, `internal/app/root.go` |
| `internal/mcp/attribution_tools.go` | Agent E | Creates | `internal/store/attribution.go`, `internal/mcp/jsonrpc.go` (Server, toolDef) |
| `internal/mcp/tools.go` | Agent E | Modifies (add `addAttributionTools(s)` call) | — |
| `internal/store/migrations.go` | Orchestrator | Modifies (add migrateV3, bump const) | `internal/store/experiment.go` (must exist first) |

---

## Wave Structure

### Wave 1 (parallel, no dependencies)

- **Agent A** — Store: attribution + replay
- **Agent B** — Store: experiment + analyzer

### Wave 2 (parallel, depends on Wave 1 completing)

- **Agent C** — App CLI: `attribute` and `replay` subcommands (depends on A)
- **Agent D** — App CLI: `experiment` subcommand (depends on B)
- **Agent E** — MCP: `get_cost_attribution` tool + wire-up (depends on A)

### Post-Wave Orchestrator Steps

1. Apply schema migration v3: add `migrateV3()` to `internal/store/migrations.go` and bump `currentSchemaVersion` to 3.
2. Run `go build ./...`
3. Run `go vet ./...`
4. Run `go test ./internal/store/... ./internal/analyzer/... ./internal/app/... ./internal/mcp/...`

---

## Agent Prompts

### Agent A — Store: attribution + replay

```
ROLE: You are a Go implementation agent working on the claudewatch codebase at
/Users/dayna.blackwell/code/claudewatch. You own exactly two new files:
internal/store/attribution.go and internal/store/replay.go. Do not touch any
other files.

GOAL: Implement the store-layer functions for per-turn cost attribution and
session replay.

FILE 1: internal/store/attribution.go
Package: store
Imports needed: bufio, encoding/json, fmt, os, path/filepath, sort, strings,
  time, github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/claude

Types to define:
  TurnAttribution struct {
    ToolType     string  `json:"tool_type"`
    Calls        int     `json:"calls"`
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    EstCostUSD   float64 `json:"est_cost_usd"`
  }

Function to implement:
  func ComputeAttribution(sessionID, claudeHome string, pricing analyzer.ModelPricing) ([]TurnAttribution, error)

IMPLEMENTATION NOTES for ComputeAttribution:
- If sessionID is non-empty, locate the JSONL at
  claudeHome/projects/<project_hash>/<sessionID>.jsonl. Walk all project
  directories under claudeHome/projects/ to find the matching file.
- If sessionID is empty, find the most recently modified .jsonl under
  claudeHome/projects/**/*.jsonl.
- Read the file with bufio.Scanner (buffer size 10MB, same as ParseSingleTranscript).
- For each line, unmarshal into claude.TranscriptEntry.
- For entries with Type == "assistant" and non-nil Message:
  - Unmarshal Message into a local struct that captures:
      Usage struct { InputTokens int; OutputTokens int } `json:"usage"`
      Content []struct { Type string; Name string } `json:"content"`
    (Do NOT import or call assistantMsgUsage from internal/claude — define a
    local unexported struct in this file.)
  - For each content block with Type == "tool_use", increment the call count
    for block.Name in an accumulator map.
  - If no tool_use blocks exist in the message, increment "text" as a pseudo
    tool type (representing pure text turns).
  - Add InputTokens and OutputTokens to the accumulator for the first (or
    dominant) tool_use type in that turn. If multiple tool types appear in one
    turn, attribute the turn's tokens to the first tool_use block's Name.
    Rationale: token counts are per-turn, not per-tool-use; assign to first.
- After walking all entries, compute EstCostUSD for each tool type:
    estCost = float64(inputTokens)/1_000_000 * pricing.InputPerMillion +
              float64(outputTokens)/1_000_000 * pricing.OutputPerMillion
- Return a []TurnAttribution sorted by EstCostUSD descending.
- Return (nil, error) only on file read failure; return empty slice if no
  attributable entries are found.

FILE 2: internal/store/replay.go
Package: store
Imports needed: bufio, encoding/json, fmt, os, path/filepath, strings,
  github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/claude

Types to define:
  ReplayTurn struct {
    Turn         int     `json:"turn"`
    Role         string  `json:"role"`
    ToolName     string  `json:"tool_name"`
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    EstCostUSD   float64 `json:"est_cost_usd"`
    Friction     bool    `json:"friction"`
    Timestamp    string  `json:"timestamp"`
  }
  SessionReplay struct {
    SessionID     string       `json:"session_id"`
    TotalTurns    int          `json:"total_turns"`
    TotalCostUSD  float64      `json:"total_cost_usd"`
    FrictionCount int          `json:"friction_count"`
    Turns         []ReplayTurn `json:"turns"`
  }

Function to implement:
  func BuildReplay(sessionID, claudeHome string, from, to int, pricing analyzer.ModelPricing) (SessionReplay, error)

IMPLEMENTATION NOTES for BuildReplay:
- Locate the JSONL file by sessionID the same way as ComputeAttribution.
- Read line by line (bufio.Scanner, 10MB buffer).
- Assign turn numbers starting at 1, incrementing on each "user" or "assistant"
  entry.
- For "assistant" entries: extract InputTokens/OutputTokens from Usage field
  (same local struct approach as attribution.go). Extract first tool_use block
  name as ToolName. Friction = false for assistant turns. Compute EstCostUSD.
- For "user" entries: look for tool_result blocks with IsError == true; if any
  found, set Friction = true. InputTokens/OutputTokens = 0 for user turns.
  ToolName = "" for user turns unless you want to show which tool result it is
  (leave as "" for simplicity). EstCostUSD = 0.
- Apply from/to bounds (1-indexed inclusive): skip turns before from, stop
  after to (0 means no bound).
- Populate SessionReplay.TotalTurns, TotalCostUSD, FrictionCount from the
  returned slice (not all turns, just the bounded slice — use pre-bound counts
  for session-level totals so the summary always reflects the full session).
  Actually: compute session totals before applying the from/to slice so the
  header shows full-session stats even when viewing a slice.
- Return (SessionReplay{}, error) on file read failure.

VERIFICATION:
After writing both files, run from /Users/dayna.blackwell/code/claudewatch:
  go build ./internal/store/...
  go vet ./internal/store/...
Both must succeed with no errors. Do not run broader tests — those are the
orchestrator's responsibility.
```

---

### Agent B — Store: experiment + analyzer

```
ROLE: You are a Go implementation agent working on the claudewatch codebase at
/Users/dayna.blackwell/code/claudewatch. You own exactly two new files:
internal/store/experiment.go and internal/analyzer/experiment.go. Do not touch
any other files.

GOAL: Implement the store layer for A/B experiments and the analyzer that
computes per-variant outcome metrics.

FILE 1: internal/store/experiment.go
Package: store
Imports needed: database/sql, fmt, time

The DB struct is defined in internal/store/db.go. Your methods are added to it.

Types to define:
  Experiment struct {
    ID        int64      `json:"id"`
    Project   string     `json:"project"`
    StartedAt time.Time  `json:"started_at"`
    StoppedAt *time.Time `json:"stopped_at,omitempty"`
    Active    bool       `json:"active"`
    Note      string     `json:"note,omitempty"`
  }
  ExperimentSession struct {
    ExperimentID int64  `json:"experiment_id"`
    SessionID    string `json:"session_id"`
    Variant      string `json:"variant"` // "a" or "b"
  }

DB methods to implement:
  func (db *DB) CreateExperiment(project, note string) (int64, error)
  func (db *DB) GetActiveExperiment(project string) (*Experiment, error)
  func (db *DB) StopExperiment(id int64) error
  func (db *DB) RecordSessionVariant(experimentID int64, sessionID, variant string) error
  func (db *DB) GetExperimentSessions(experimentID int64) ([]ExperimentSession, error)
  func (db *DB) ListExperiments(project string) ([]Experiment, error)

IMPLEMENTATION NOTES:
- Do NOT add the schema migration here. That is the orchestrator's job. Your
  code must assume the following tables exist (they will be created by the
  orchestrator's migrateV3 before any agent code runs in production; for your
  build/vet check, the DB methods just need to compile):

    CREATE TABLE IF NOT EXISTS experiments (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        project    TEXT    NOT NULL,
        started_at TEXT    NOT NULL,  -- RFC3339
        stopped_at TEXT,              -- NULL if active
        active     BOOLEAN NOT NULL DEFAULT true,
        note       TEXT    NOT NULL DEFAULT ''
    );
    CREATE TABLE IF NOT EXISTS experiment_sessions (
        experiment_id INTEGER NOT NULL REFERENCES experiments(id),
        session_id    TEXT    NOT NULL,
        variant       TEXT    NOT NULL,
        PRIMARY KEY (experiment_id, session_id)
    );

- CreateExperiment: INSERT into experiments with active=true, started_at=time.Now().UTC().
  If a previous active experiment exists for the project, return an error
  "active experiment already exists for project %q — stop it first".
- GetActiveExperiment: SELECT WHERE project=? AND active=1 ORDER BY started_at DESC LIMIT 1.
  Return nil, nil if not found.
- StopExperiment: UPDATE experiments SET active=false, stopped_at=? WHERE id=?.
- RecordSessionVariant: INSERT OR REPLACE into experiment_sessions.
- GetExperimentSessions: SELECT WHERE experiment_id=?.
- ListExperiments: SELECT WHERE project=? ORDER BY started_at DESC (all, not just active).
- Use time.RFC3339 for all timestamp formatting/scanning.
- For started_at/stopped_at scanning, scan as string then parse with
  time.Parse(time.RFC3339, s). For stopped_at, scan into *string and convert.

FILE 2: internal/analyzer/experiment.go
Package: analyzer
Imports needed: fmt, math, sort,
  github.com/blackwell-systems/claudewatch/internal/claude,
  github.com/blackwell-systems/claudewatch/internal/store

Types to define:
  VariantStats struct {
    Variant      string  `json:"variant"`
    SessionCount int     `json:"session_count"`
    AvgCostUSD   float64 `json:"avg_cost_usd"`
    AvgFriction  float64 `json:"avg_friction"`
    AvgCommits   float64 `json:"avg_commits"`
  }
  ExperimentReport struct {
    ExperimentID int64        `json:"experiment_id"`
    Project      string       `json:"project"`
    A            VariantStats `json:"variant_a"`
    B            VariantStats `json:"variant_b"`
    Winner       string       `json:"winner"`     // "a", "b", or "inconclusive"
    Confidence   string       `json:"confidence"` // "high", "low", "inconclusive"
    Summary      string       `json:"summary"`
  }

Function to implement:
  func AnalyzeExperiment(
      exp store.Experiment,
      sessions []claude.SessionMeta,
      facets []claude.SessionFacet,
      assignments map[string]string, // sessionID → "a" or "b"
      pricing ModelPricing,
      ratio CacheRatio,
  ) ExperimentReport

IMPLEMENTATION NOTES for AnalyzeExperiment:
- Build a facet index by sessionID (same pattern as in analyzer/outcomes.go).
- Partition sessions into variant A and variant B lists using assignments map.
- For each variant, compute:
    AvgCostUSD:  mean of EstimateSessionCost(s, pricing, ratio) per session
    AvgFriction: mean of sum(facet.FrictionCounts values) per session (0 if no facet)
    AvgCommits:  mean of s.GitCommits per session
- Winner determination (compare primary metric = AvgCostUSD, lower is better;
  secondary = AvgFriction, lower is better):
    If len(A) < 5 || len(B) < 5: Winner = "inconclusive", Confidence = "inconclusive"
    Else: compare AvgCostUSD; if difference > 10% of the higher value, declare
    the lower-cost variant the winner with Confidence "high" if N >= 10 per
    variant, "low" otherwise.
    If cost difference <= 10%: compare AvgFriction; same threshold logic.
    If both within threshold: Winner = "inconclusive", Confidence = "low".
- Build a human-readable Summary string, e.g.:
    "Variant A: 8 sessions, avg $0.42, friction 2.1, commits 1.8. " +
    "Variant B: 9 sessions, avg $0.31, friction 1.7, commits 2.1. " +
    "Winner: B (cost 26% lower)."
- EstimateSessionCost is defined in internal/analyzer/outcomes.go (same package).
  Call it directly.
- The mean helper is defined in internal/analyzer/anomaly.go (same package).
  Call it directly.

VERIFICATION:
After writing both files, run from /Users/dayna.blackwell/code/claudewatch:
  go build ./internal/store/...
  go build ./internal/analyzer/...
  go vet ./internal/store/...
  go vet ./internal/analyzer/...
All must succeed with no errors.
```

---

### Agent C — App CLI: attribute + replay subcommands

```
ROLE: You are a Go implementation agent working on the claudewatch codebase at
/Users/dayna.blackwell/code/claudewatch. You own exactly two new files:
internal/app/attribute.go and internal/app/replay.go. Do not modify any
existing files.

GOAL: Implement the `claudewatch attribute` and `claudewatch replay` CLI
subcommands as Cobra commands registered on rootCmd.

PREREQUISITE: Wave 1 must have completed. internal/store/attribution.go and
internal/store/replay.go must exist and compile before you begin.

FILE 1: internal/app/attribute.go
Package: app

Imports: encoding/json, fmt, os, sort, strings,
  github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/claude,
  github.com/blackwell-systems/claudewatch/internal/config,
  github.com/blackwell-systems/claudewatch/internal/output,
  github.com/blackwell-systems/claudewatch/internal/store,
  github.com/spf13/cobra

Variables (package-level, prefixed to avoid collision):
  attrFlagSession string
  attrFlagProject string  (reserved for future multi-session filtering; not functional in v1)

Command definition:
  attributeCmd = &cobra.Command{
    Use:   "attribute",
    Short: "Break down token cost by tool type for a session",
    Long: `Show which tool types consumed most tokens and budget in a session.
Defaults to the most recent session.

Examples:
  claudewatch attribute
  claudewatch attribute --session abc123
  claudewatch attribute --json`,
    RunE: runAttribute,
  }

init():
  attributeCmd.Flags().StringVar(&attrFlagSession, "session", "", "Session ID to analyze (default: most recent)")
  rootCmd.AddCommand(attributeCmd)

runAttribute implementation:
  1. Load config with config.Load(flagConfig).
  2. Load pricing: analyzer.DefaultPricing["sonnet"].
  3. sessionID := attrFlagSession (may be empty — ComputeAttribution handles empty).
  4. Call store.ComputeAttribution(sessionID, cfg.ClaudeHome, pricing).
  5. If flagJSON: json.NewEncoder(os.Stdout).Encode(rows); return nil.
  6. Else render table with output.NewTable("Tool Type", "Calls", "Input Tokens", "Output Tokens", "Est. Cost").
     Format EstCostUSD as "$%.4f". Format tokens with comma separators via
     fmt.Sprintf("%d", n) (no special formatting required — keep it simple).
     Print a summary line at the bottom: "Total: $X.XXXX"
  7. Use fmt.Println(output.Section("Cost Attribution")) as the header.

FILE 2: internal/app/replay.go
Package: app

Imports: encoding/json, fmt, os,
  github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/config,
  github.com/blackwell-systems/claudewatch/internal/output,
  github.com/blackwell-systems/claudewatch/internal/store,
  github.com/spf13/cobra

Variables:
  replayFlagFrom int
  replayFlagTo   int

Command definition:
  replayCmd = &cobra.Command{
    Use:   "replay <session-id>",
    Short: "Walk through a session as a structured timeline",
    Long: `Show every turn in a session with role, tool, token usage, cost, and
friction markers. Useful for post-mortems on expensive or high-friction sessions.

Examples:
  claudewatch replay abc123def456
  claudewatch replay abc123 --from 10 --to 20
  claudewatch replay abc123 --json`,
    Args: cobra.ExactArgs(1),
    RunE: runReplay,
  }

init():
  replayCmd.Flags().IntVar(&replayFlagFrom, "from", 0, "First turn to show (1-indexed, default: all)")
  replayCmd.Flags().IntVar(&replayFlagTo,   "to",   0, "Last turn to show (1-indexed, default: all)")
  rootCmd.AddCommand(replayCmd)

runReplay implementation:
  1. Load config.
  2. Load pricing: analyzer.DefaultPricing["sonnet"].
  3. sessionID := args[0].
  4. Call store.BuildReplay(sessionID, cfg.ClaudeHome, replayFlagFrom, replayFlagTo, pricing).
  5. If flagJSON: encode and return.
  6. Else:
     - Print output.Section(fmt.Sprintf("Session Replay — %s", sessionID[:min(12, len(sessionID))]))
     - Print summary line: "%d turns | $%.4f total | %d friction events"
     - Render table with output.NewTable("Turn", "Role", "Tool", "In Tok", "Out Tok", "Cost", "F").
       For Friction column: "!" if true, "" if false.
       For ToolName: truncate to 20 chars if longer.
       Format Cost as "$%.4f".

IMPORTANT: Do not define a new `min` function. Package `app` already has one in
doctor.go, and Go 1.26 has a built-in. Use the built-in min(a, b int).

VERIFICATION:
After writing both files, run from /Users/dayna.blackwell/code/claudewatch:
  go build ./internal/app/...
  go vet ./internal/app/...
Both must succeed with no errors.
```

---

### Agent D — App CLI: experiment subcommand

```
ROLE: You are a Go implementation agent working on the claudewatch codebase at
/Users/dayna.blackwell/code/claudewatch. You own exactly one new file:
internal/app/experiment.go. Do not modify any existing files.

GOAL: Implement the `claudewatch experiment` CLI subcommand.

PREREQUISITE: Wave 1 must have completed. internal/store/experiment.go and
internal/analyzer/experiment.go must exist and compile before you begin.

FILE: internal/app/experiment.go
Package: app

Imports: encoding/json, fmt, os, sort,
  github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/claude,
  github.com/blackwell-systems/claudewatch/internal/config,
  github.com/blackwell-systems/claudewatch/internal/output,
  github.com/blackwell-systems/claudewatch/internal/store,
  github.com/spf13/cobra

Top-level command:
  experimentCmd = &cobra.Command{
    Use:   "experiment",
    Short: "Manage CLAUDE.md A/B experiments",
    Long: `Create and report on A/B experiments that compare two CLAUDE.md variants.

Subcommands: start, stop, tag, report`,
  }
  func init() { rootCmd.AddCommand(experimentCmd) }

Subcommand: experiment start
  Flags: --project string (required), --note string
  Action: Load config. Open DB. Call db.CreateExperiment(project, note).
          Print: "Experiment #%d started for project %q."
          If error (e.g. active experiment already exists): return the error.

Subcommand: experiment stop
  Flags: --project string (required)
  Action: Load config. Open DB. GetActiveExperiment(project).
          If nil: return fmt.Errorf("no active experiment for project %q", project).
          Call db.StopExperiment(exp.ID). Print: "Experiment #%d stopped."

Subcommand: experiment tag
  Flags: --project string (required), --session string (default: most recent),
         --variant string (required, must be "a" or "b")
  Action:
    1. Load config. Open DB.
    2. GetActiveExperiment(project). Return error if none.
    3. If --session empty: load sessions via claude.ParseAllSessionMeta, sort
       descending, take sessions[0].SessionID.
    4. Validate variant is "a" or "b"; return error otherwise.
    5. db.RecordSessionVariant(exp.ID, sessionID, variant).
    6. Print: "Tagged session %s as variant %s in experiment #%d."

Subcommand: experiment report
  Flags: --project string (required), --json
  Action:
    1. Load config. Open DB.
    2. GetActiveExperiment(project). If nil, try ListExperiments(project)[0] as fallback
       (most recent stopped experiment). Return error if no experiments at all.
    3. GetExperimentSessions(exp.ID) → build assignments map[sessionID]variant.
    4. ParseAllSessionMeta → filter to sessions in assignments map.
    5. ParseAllFacets → filter to sessions in assignments map.
    6. Load pricing = analyzer.DefaultPricing["sonnet"], ratio = analyzer.NoCacheRatio().
       Attempt to load better ratio via claude.ParseStatsCache(cfg.ClaudeHome).
    7. Call analyzer.AnalyzeExperiment(exp, filteredSessions, filteredFacets,
       assignments, pricing, ratio).
    8. If flagJSON: encode report and return.
    9. Else render:
         output.Section("Experiment Report — <project> #<id>")
         Two-column table: Metric | Variant A | Variant B
           Sessions, Avg Cost, Avg Friction, Avg Commits
         Winner line: "Winner: A — cost 15% lower" (or "inconclusive")

    ParseAllFacets is in internal/claude/facets.go — call claude.ParseAllFacets(cfg.ClaudeHome).

DB open pattern (same as other app commands):
  db, err := store.Open(config.DBPath())
  if err != nil { return fmt.Errorf("opening database: %w", err) }
  defer func() { _ = db.Close() }()

VERIFICATION:
After writing the file, run from /Users/dayna.blackwell/code/claudewatch:
  go build ./internal/app/...
  go vet ./internal/app/...
Both must succeed with no errors.
```

---

### Agent E — MCP: get_cost_attribution tool

```
ROLE: You are a Go implementation agent working on the claudewatch codebase at
/Users/dayna.blackwell/code/claudewatch. You own exactly one new file:
internal/mcp/attribution_tools.go. You also make one small addition to
internal/mcp/tools.go (one line in addTools).

GOAL: Register the get_cost_attribution MCP tool.

PREREQUISITE: Wave 1 must have completed. internal/store/attribution.go must
exist and compile before you begin.

FILE 1 (new): internal/mcp/attribution_tools.go
Package: mcp

Imports: encoding/json,
  github.com/blackwell-systems/claudewatch/internal/analyzer,
  github.com/blackwell-systems/claudewatch/internal/config,
  github.com/blackwell-systems/claudewatch/internal/store

// CostAttributionResult wraps the attribution rows for MCP response.
type CostAttributionResult struct {
    SessionID string                  `json:"session_id"`
    Rows      []store.TurnAttribution `json:"rows"`
    TotalCost float64                 `json:"total_cost_usd"`
}

// addAttributionTools registers get_cost_attribution on s.
func addAttributionTools(s *Server) {
    s.registerTool(toolDef{
        Name:        "get_cost_attribution",
        Description: "Break down token cost by tool type for a session. Answer 'which tool calls consumed most of my budget?' Defaults to most recent session.",
        InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to analyze (optional, defaults to most recent)"},"project":{"type":"string","description":"Project name filter (optional, reserved for future use)"}},"additionalProperties":false}`),
        Handler:     s.handleGetCostAttribution,
    })
}

// handleGetCostAttribution computes per-tool-type attribution for a session.
func (s *Server) handleGetCostAttribution(args json.RawMessage) (any, error) {
    var params struct {
        SessionID *string `json:"session_id"`
        Project   *string `json:"project"`
    }
    if len(args) > 0 && string(args) != "null" {
        _ = json.Unmarshal(args, &params)
    }

    sessionID := ""
    if params.SessionID != nil {
        sessionID = *params.SessionID
    }

    pricing := analyzer.DefaultPricing["sonnet"]
    rows, err := store.ComputeAttribution(sessionID, s.claudeHome, pricing)
    if err != nil {
        return nil, err
    }
    if rows == nil {
        rows = []store.TurnAttribution{}
    }

    var total float64
    for _, r := range rows {
        total += r.EstCostUSD
    }

    resolvedID := sessionID
    if resolvedID == "" {
        resolvedID = "most-recent"
    }

    return CostAttributionResult{
        SessionID: resolvedID,
        Rows:      rows,
        TotalCost: total,
    }, nil
}

FILE 2 (modify): internal/mcp/tools.go
In the addTools function, add the following line BEFORE the closing brace,
immediately after the existing addTranscriptTools(s) call or after addAnomalyTools(s):

    addAttributionTools(s)

Read the file first to find the exact insertion point. The file ends with:
    addTranscriptTools(s)
    addAnomalyTools(s)
    s.registerTool(toolDef{ ... "get_project_comparison" ... })
    s.registerTool(toolDef{ ... "get_stale_patterns" ... })
}

Add addAttributionTools(s) after addAnomalyTools(s) and before the
get_project_comparison registration, OR after get_stale_patterns (either
position is correct — pick the one that is cleanest).

VERIFICATION:
After writing attribution_tools.go and modifying tools.go, run from
/Users/dayna.blackwell/code/claudewatch:
  go build ./internal/mcp/...
  go vet ./internal/mcp/...
Both must succeed with no errors.
```

---

## Orchestrator Post-Merge Steps

After all five agents complete and their files are verified:

### Step 1: Apply schema migration v3

Edit `internal/store/migrations.go`:

1. Change `const currentSchemaVersion = 2` to `const currentSchemaVersion = 3`.

2. In the `Migrate()` function, add after the `version < 2` block:
   ```go
   if version < 3 {
       if err := db.migrateV3(); err != nil {
           return fmt.Errorf("migration v3: %w", err)
       }
   }
   ```

3. Add the `migrateV3()` method:
   ```go
   // migrateV3 adds the experiments and experiment_sessions tables.
   func (db *DB) migrateV3() error {
       statements := []string{
           `CREATE TABLE IF NOT EXISTS experiments (
               id         INTEGER PRIMARY KEY AUTOINCREMENT,
               project    TEXT    NOT NULL,
               started_at TEXT    NOT NULL,
               stopped_at TEXT,
               active     BOOLEAN NOT NULL DEFAULT 1,
               note       TEXT    NOT NULL DEFAULT ''
           )`,
           `CREATE TABLE IF NOT EXISTS experiment_sessions (
               experiment_id INTEGER NOT NULL REFERENCES experiments(id),
               session_id    TEXT    NOT NULL,
               variant       TEXT    NOT NULL,
               PRIMARY KEY (experiment_id, session_id)
           )`,
           `CREATE INDEX IF NOT EXISTS idx_experiments_project ON experiments(project)`,
           `CREATE INDEX IF NOT EXISTS idx_experiments_active ON experiments(active)`,
           `CREATE INDEX IF NOT EXISTS idx_exp_sessions_exp ON experiment_sessions(experiment_id)`,
       }

       tx, err := db.conn.Begin()
       if err != nil {
           return err
       }
       defer func() { _ = tx.Rollback() }()

       for _, stmt := range statements {
           if _, err := tx.Exec(stmt); err != nil {
               l := len(stmt)
               if l > 40 {
                   l = 40
               }
               return fmt.Errorf("executing %q: %w", stmt[:l], err)
           }
       }

       if _, err := tx.Exec("DELETE FROM schema_version"); err != nil {
           return err
       }
       if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", 3); err != nil {
           return err
       }

       return tx.Commit()
   }
   ```

### Step 2: Full build and vet

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
```

Both must pass with zero errors.

### Step 3: Focused tests

```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/store/...
go test ./internal/analyzer/...
go test ./internal/app/...
go test ./internal/mcp/...
```

### Step 4: Smoke test (optional, if binary builds)

```bash
cd /Users/dayna.blackwell/code/claudewatch
go run ./cmd/claudewatch attribute --help
go run ./cmd/claudewatch replay --help
go run ./cmd/claudewatch experiment --help
```

---

## Status Checklist

### Wave 1

- [ ] **Agent A** — `internal/store/attribution.go` created
- [ ] **Agent A** — `internal/store/replay.go` created
- [ ] **Agent A** — `go build ./internal/store/...` passes
- [ ] **Agent A** — `go vet ./internal/store/...` passes
- [ ] **Agent B** — `internal/store/experiment.go` created
- [ ] **Agent B** — `internal/analyzer/experiment.go` created
- [ ] **Agent B** — `go build ./internal/store/...` passes
- [ ] **Agent B** — `go build ./internal/analyzer/...` passes
- [ ] **Agent B** — `go vet ./internal/store/...` passes
- [ ] **Agent B** — `go vet ./internal/analyzer/...` passes

### Wave 2

- [ ] **Agent C** — `internal/app/attribute.go` created
- [ ] **Agent C** — `internal/app/replay.go` created
- [ ] **Agent C** — `go build ./internal/app/...` passes
- [ ] **Agent C** — `go vet ./internal/app/...` passes
- [ ] **Agent D** — `internal/app/experiment.go` created
- [ ] **Agent D** — `go build ./internal/app/...` passes
- [ ] **Agent D** — `go vet ./internal/app/...` passes
- [ ] **Agent E** — `internal/mcp/attribution_tools.go` created
- [ ] **Agent E** — `internal/mcp/tools.go` modified (addAttributionTools call added)
- [ ] **Agent E** — `go build ./internal/mcp/...` passes
- [ ] **Agent E** — `go vet ./internal/mcp/...` passes

### Orchestrator

- [ ] `internal/store/migrations.go` — `currentSchemaVersion` bumped to 3
- [ ] `internal/store/migrations.go` — `migrateV3()` added
- [ ] `internal/store/migrations.go` — `Migrate()` calls `migrateV3()` for `version < 3`
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `go test ./internal/store/...` passes
- [ ] `go test ./internal/analyzer/...` passes
- [ ] `go test ./internal/app/...` passes
- [ ] `go test ./internal/mcp/...` passes
