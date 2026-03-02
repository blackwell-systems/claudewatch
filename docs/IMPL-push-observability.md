# IMPL: Push-Based Observability

**Feature:** Evolve claudewatch from pull-based MCP tools to push-based hooks
that actively surface friction mid-session, plus new MCP tools for context
pressure and cost velocity, and smarter friction pattern classification.

**Repo:** `/Users/dayna.blackwell/code/claudewatch` (Go)

---

### Suitability Assessment

Verdict: SUITABLE

The work decomposes into 4 independent tasks across disjoint files:

1. **Friction alert hook** -- new shell script (`bin/friction-alert-hook`) and a
   new Go subcommand (`internal/app/hook.go`) that provides the PostToolUse
   hook logic. Touches no existing files except `internal/app/root.go` (to
   register the subcommand).
2. **`get_context_pressure` MCP tool** -- new file `internal/mcp/context_tools.go`
   plus new parsing function in `internal/claude/active_live.go`.
3. **`get_cost_velocity` MCP tool** -- new file `internal/mcp/cost_velocity_tools.go`
   plus new parsing function in `internal/claude/active_live.go`.
4. **Friction pattern classification** -- modify existing `internal/claude/active_live.go`
   (ParseLiveFriction) and `internal/mcp/live_tools.go` (LiveFrictionResult).

Items 2, 3, and 4 all touch `internal/claude/active_live.go`, which creates
an ownership conflict. Resolution: Items 2 and 3 add **new, independent
functions** to `active_live.go` (they do not modify existing functions), so
they can be split into separate agents only if one completes before the other.
Item 4 modifies the **existing** `ParseLiveFriction` function. Therefore:

- Agent A (hook): fully independent, Wave 1.
- Agent B (context pressure): new function in `active_live.go` + new MCP file. Wave 1.
- Agent C (cost velocity): new function in `active_live.go` + new MCP file. Wave 1,
  but shares `active_live.go` with B. Resolution: Agent C adds its function to a
  **new file** `internal/claude/active_live_cost.go` to avoid ownership conflict.
- Agent D (friction patterns): modifies existing code in `active_live.go` and
  `live_tools.go`. Wave 1, shares `active_live.go` with B. Resolution: B adds
  its function to a **new file** `internal/claude/active_live_context.go`.

With file splitting, all 4 agents have disjoint file ownership and can run in
a single wave.

**Suitability gate answers:**

1. **File decomposition:** 4 agents, disjoint after splitting `active_live.go`
   additions into new files. The shared file `internal/mcp/tools.go` is
   orchestrator-owned (append-only registration calls added post-merge).
2. **Investigation-first items:** None. All requirements are clearly specified
   with known interfaces.
3. **Interface discoverability:** Yes. All cross-agent interfaces are function
   signatures in the `claude` package, fully definable upfront.
4. **Pre-implementation status check:** All 4 items are TO-DO (none exist yet).
5. **Parallelization value check:** `go test ./...` takes ~8 seconds. 4 agents
   each touching 2-3 files with non-trivial logic. Single wave = maximum
   parallelism. Clear speedup.

```
Pre-implementation scan results:
- Total items: 4 features
- Already implemented: 0 items (0% of work)
- Partially implemented: 0 items
- To-do: 4 items

Agent adjustments: None needed (all to-do).
```

```
Estimated times:
- Scout phase: ~10 min (dependency mapping, interface contracts, IMPL doc)
- Agent execution: ~8 min (4 agents x 8 min avg, all parallel in 1 wave)
- Merge & verification: ~5 min
Total SAW time: ~23 min

Sequential baseline: ~32 min (4 agents x 8 min avg)
Time savings: ~9 min (28% faster)

Recommendation: Clear speedup. 4 fully parallel agents with non-trivial logic
and an 8-second build/test cycle.
```

### Known Issues

None identified. All tests pass cleanly (`go test ./...`).

---

### Dependency Graph

```
                    [orchestrator: tools.go registration]
                         ^    ^    ^    ^
                         |    |    |    |
    [Agent A]       [Agent B]  [Agent C]  [Agent D]
    hook.go         context    cost_vel   friction
    hook script     _tools.go  _tools.go  patterns
                    context.go cost.go    active_live.go
                                          live_tools.go
```

All four agents are leaf nodes -- none depends on another agent's output.
The orchestrator owns `internal/mcp/tools.go` for post-merge registration
of new tools (append-only).

**Root nodes:** None (no prerequisites).
**Leaf nodes:** All four agents.

**Cascade candidates** (files not in any agent's scope but referencing changed
interfaces):
- `internal/mcp/tools.go` -- orchestrator will add `addContextTools(s)`,
  `addCostVelocityTools(s)` calls post-merge. Not modified by any agent.
- `internal/mcp/jsonrpc_test.go` -- tests tool list count; will need count
  update post-merge.

---

### Interface Contracts

#### Agent A: Hook subcommand

New function in `internal/app/hook.go`:
```go
// hookCmd returns a *cobra.Command for the "hook" subcommand.
// Usage: claudewatch hook --type PostToolUse
// It reads the active session, checks friction thresholds, and prints
// a warning to stdout if thresholds are crossed. Exits 0 always.
func hookCmd() *cobra.Command
```

New shell script `bin/friction-alert-hook`:
```bash
#!/bin/bash
# PostToolUse hook for Claude Code.
# Invokes: claudewatch hook --type PostToolUse
# Prints warning message to stdout if friction thresholds crossed.
# Exit 0 always (hooks must not block).
```

Consumes from `internal/claude` (existing, no changes):
```go
func FindActiveSessionPath(claudeHome string) (string, error)
func ParseLiveToolErrors(path string) (*LiveToolErrorStats, error)
```

#### Agent B: Context pressure

New file `internal/claude/active_live_context.go`:
```go
// ContextPressureStats holds context window utilization data.
type ContextPressureStats struct {
    TotalInputTokens  int     `json:"total_input_tokens"`
    TotalOutputTokens int     `json:"total_output_tokens"`
    TotalTokens       int     `json:"total_tokens"`
    Compactions       int     `json:"compactions"`
    EstimatedUsage    float64 `json:"estimated_usage"`   // 0.0-1.0
    Status            string  `json:"status"`            // "comfortable","filling","pressure","critical"
}

// ParseLiveContextPressure reads the JSONL file at path and computes
// context window utilization from cumulative token usage and compaction events.
func ParseLiveContextPressure(path string) (*ContextPressureStats, error)
```

New file `internal/mcp/context_tools.go`:
```go
// ContextPressureResult holds the MCP response for get_context_pressure.
type ContextPressureResult struct {
    SessionID         string  `json:"session_id"`
    Live              bool    `json:"live"`
    TotalInputTokens  int     `json:"total_input_tokens"`
    TotalOutputTokens int     `json:"total_output_tokens"`
    TotalTokens       int     `json:"total_tokens"`
    Compactions       int     `json:"compactions"`
    EstimatedUsage    float64 `json:"estimated_usage"`
    Status            string  `json:"status"`
}

// addContextTools registers the get_context_pressure handler on s.
func addContextTools(s *Server)
```

#### Agent C: Cost velocity

New file `internal/claude/active_live_cost.go`:
```go
// LiveCostVelocityStats holds cost-per-minute data for a rolling window.
type LiveCostVelocityStats struct {
    WindowMinutes  float64 `json:"window_minutes"`
    WindowCostUSD  float64 `json:"window_cost_usd"`
    CostPerMinute  float64 `json:"cost_per_minute"`
    Status         string  `json:"status"` // "efficient","normal","burning"
}

// ParseLiveCostVelocity reads the JSONL file and computes cost in a rolling
// window using per-turn token usage and the provided pricing.
func ParseLiveCostVelocity(path string, windowMinutes float64, pricing CostPricing) (*LiveCostVelocityStats, error)

// CostPricing holds the per-million-token rates needed for cost calculation.
// This avoids importing the analyzer package from the claude package.
type CostPricing struct {
    InputPerMillion  float64
    OutputPerMillion float64
}
```

New file `internal/mcp/cost_velocity_tools.go`:
```go
// CostVelocityResult holds the MCP response for get_cost_velocity.
type CostVelocityResult struct {
    SessionID     string  `json:"session_id"`
    Live          bool    `json:"live"`
    WindowMinutes float64 `json:"window_minutes"`
    WindowCostUSD float64 `json:"window_cost_usd"`
    CostPerMinute float64 `json:"cost_per_minute"`
    Status        string  `json:"status"`
}

// addCostVelocityTools registers the get_cost_velocity handler on s.
func addCostVelocityTools(s *Server)
```

#### Agent D: Friction patterns

Modified types in `internal/claude/active_live.go`:
```go
// FrictionPattern represents a collapsed group of similar friction events.
type FrictionPattern struct {
    Type        string `json:"type"`         // e.g. "permission_denied", "tool_error:Bash", "same_file_retry:foo.go"
    Count       int    `json:"count"`        // total occurrences
    Consecutive bool   `json:"consecutive"`  // true if all occurrences were consecutive
    FirstTurn   int    `json:"first_turn"`   // approximate turn number of first occurrence
    LastTurn    int    `json:"last_turn"`    // approximate turn number of last occurrence
}

// Add Patterns field to LiveFrictionStats:
// Patterns []FrictionPattern `json:"patterns"`
```

Modified in `internal/mcp/live_tools.go`:
```go
// Add Patterns field to LiveFrictionResult:
// Patterns []claude.FrictionPattern `json:"patterns,omitempty"`
```

---

### File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `internal/app/hook.go` (new) | A | 1 | -- |
| `internal/app/hook_test.go` (new) | A | 1 | -- |
| `bin/friction-alert-hook` (new) | A | 1 | -- |
| `internal/claude/active_live_context.go` (new) | B | 1 | -- |
| `internal/claude/active_live_context_test.go` (new) | B | 1 | -- |
| `internal/mcp/context_tools.go` (new) | B | 1 | -- |
| `internal/mcp/context_tools_test.go` (new) | B | 1 | -- |
| `internal/claude/active_live_cost.go` (new) | C | 1 | -- |
| `internal/claude/active_live_cost_test.go` (new) | C | 1 | -- |
| `internal/mcp/cost_velocity_tools.go` (new) | C | 1 | -- |
| `internal/mcp/cost_velocity_tools_test.go` (new) | C | 1 | -- |
| `internal/claude/active_live.go` (modify) | D | 1 | -- |
| `internal/claude/active_live_test.go` (modify) | D | 1 | -- |
| `internal/mcp/live_tools.go` (modify) | D | 1 | -- |
| `internal/mcp/live_tools_test.go` (modify) | D | 1 | -- |
| `internal/mcp/tools.go` (modify) | orchestrator | post-merge | A,B,C,D |
| `internal/app/root.go` (modify) | orchestrator | post-merge | A |

---

### Wave Structure

```
Wave 1: [A] [B] [C] [D]    <- 4 parallel agents, all independent
              |
         (all complete)
              |
       [orchestrator]       <- post-merge: register tools, register subcommand
```

No Wave 0 needed. No multi-wave dependencies.

---

### Agent Prompts

#### Agent A -- Friction Alert Hook

**Goal:** Create a `claudewatch hook` subcommand and a companion shell script
that Claude Code calls as a PostToolUse hook. When friction thresholds are
crossed, it prints a warning message to stdout.

**Context:** claudewatch is a Go CLI at `/Users/dayna.blackwell/code/claudewatch`.
The existing live session parsing in `internal/claude/active_live.go` provides
`ParseLiveToolErrors(path) (*LiveToolErrorStats, error)` which returns
`TotalToolUses`, `TotalErrors`, `ErrorRate`, `ErrorsByTool`, and
`ConsecutiveErrs`. The function `FindActiveSessionPath(claudeHome)` in
`internal/claude/active.go` locates the active JSONL file. The config package
at `internal/config/config.go` provides `DefaultClaudeHome()` for the default
`~/.claude` path.

**Files you own (create or modify):**
- `internal/app/hook.go` (new) -- cobra subcommand
- `internal/app/hook_test.go` (new) -- tests
- `bin/friction-alert-hook` (new) -- shell script wrapper

**Do not modify any other files.**

**Requirements:**

1. Create `internal/app/hook.go` with a `hookCmd() *cobra.Command` function:
   - Flag: `--type` (string, required) -- currently only "PostToolUse" is supported.
   - On `RunE`:
     a. Call `claude.FindActiveSessionPath(config.DefaultClaudeHome())`.
     b. If no active session, exit 0 silently.
     c. Call `claude.ParseLiveToolErrors(activePath)`.
     d. Check thresholds and print warnings to stdout:
        - If `ConsecutiveErrs >= 3`: print `claudewatch: {N} consecutive errors on {tool}. Error rate: {rate}%. Consider reading before editing.`
          where `{tool}` is the tool with the most errors in `ErrorsByTool`.
        - Else if `ErrorRate > 0.30` and `TotalToolUses >= 10`: print `claudewatch: High error rate ({rate}%). Slow down.`
        - (The "same file edited >3 times" check is deferred -- it requires
          file-level tracking not yet in ParseLiveToolErrors.)
     e. Always exit 0 (hooks must not block Claude Code).

2. Create `bin/friction-alert-hook` shell script:
   ```bash
   #!/usr/bin/env bash
   # Claude Code PostToolUse hook -- surfaces friction alerts mid-session.
   # Install: add to ~/.claude/settings.json hooks.PostToolUse
   exec claudewatch hook --type PostToolUse 2>/dev/null
   ```
   Make it executable (0755).

3. Write tests in `internal/app/hook_test.go` that verify:
   - The command exists and accepts `--type PostToolUse`.
   - Threshold logic: given mock stats, verify correct output messages.
   - Test the threshold functions directly (extract threshold checking into
     a testable function `checkFrictionThresholds(stats *claude.LiveToolErrorStats) string`).

**Interface contracts consumed:**
```go
// internal/claude/active.go (existing, do not modify)
func FindActiveSessionPath(claudeHome string) (string, error)

// internal/claude/active_live.go (existing, do not modify)
func ParseLiveToolErrors(path string) (*LiveToolErrorStats, error)

// internal/config/config.go (existing, do not modify)
func DefaultClaudeHome() string
```

**Verification gate:**
```bash
go build ./...
go vet ./...
go test ./internal/app -run TestHook -count=1 -timeout 30s
```

---

#### Agent B -- Context Pressure MCP Tool

**Goal:** Add a `get_context_pressure` MCP tool that reports context window
utilization for the active session.

**Context:** claudewatch is a Go CLI. The MCP server is in `internal/mcp/`.
Live session JSONL parsing is in `internal/claude/active_live.go`. The
`readLiveJSONL(path)` function reads entries; `ParseLiveTokenWindow` shows
the pattern for windowed token analysis. The `TranscriptEntry` type has
`Type`, `Timestamp`, `Message` fields. Assistant messages contain usage data
extractable via `assistantMsgUsage` (unexported but you can replicate the
pattern). Compaction events appear as entries with `type: "summary"` in the
JSONL (these are injected when Claude Code compacts context).

**Files you own (create):**
- `internal/claude/active_live_context.go` (new)
- `internal/claude/active_live_context_test.go` (new)
- `internal/mcp/context_tools.go` (new)
- `internal/mcp/context_tools_test.go` (new)

**Do not modify any other files.**

**Requirements:**

1. Create `internal/claude/active_live_context.go`:
   - Define `ContextPressureStats` struct (see Interface Contracts above).
   - Implement `ParseLiveContextPressure(path string) (*ContextPressureStats, error)`:
     a. Call `readLiveJSONL(path)` to get entries.
     b. Sum input and output tokens from assistant messages (same pattern as
        `ParseActiveSession` -- unmarshal `assistantMsgUsage` from `entry.Message`).
     c. Count compaction events: entries where `entry.Type == "summary"`.
     d. Estimate context usage: use the most recent assistant message's
        cumulative input tokens as the current context size. Claude Code's
        context window is 200K tokens. `EstimatedUsage = lastInputTokens / 200000.0`.
        Note: `lastInputTokens` is the input_tokens from the most recent
        assistant entry's usage (this represents the full context sent in the
        last turn, which is the best proxy for current context size).
     e. Compute status:
        - `< 0.5` -> "comfortable"
        - `0.5 - 0.75` -> "filling"
        - `0.75 - 0.9` -> "pressure"
        - `>= 0.9` -> "critical"

2. Create `internal/mcp/context_tools.go`:
   - Define `ContextPressureResult` struct (see Interface Contracts).
   - Implement `addContextTools(s *Server)` that registers `get_context_pressure`.
   - Handler: find active session, call `ParseLiveContextPressure`, return result.
   - Description: "How much of the context window has been consumed. Reports
     token usage, compaction count, and utilization status (comfortable/filling/
     pressure/critical)."

3. Write comprehensive tests:
   - `active_live_context_test.go`: test `ParseLiveContextPressure` with mock
     JSONL data. Use the existing `mkAssistantToolUse` and `writeLiveJSONL`
     test helpers from `active_live_test.go` (they are in the same package).
     Also create helper entries with usage data. Test: no entries, low usage,
     high usage with compaction events.
   - `context_tools_test.go`: test the MCP handler integration using the
     pattern from `live_tools_test.go`. Use `NewServer` + direct handler call.

**Interface contracts consumed:**
```go
// internal/claude/active_live.go (existing, do not modify)
func readLiveJSONL(path string) ([]TranscriptEntry, error)  // unexported, same package

// internal/claude/transcripts.go (existing, do not modify)
type TranscriptEntry struct { ... }
type assistantMsgUsage struct { ... }  // unexported, same package -- replicate pattern
```

**Verification gate:**
```bash
go build ./...
go vet ./...
go test ./internal/claude -run TestParseLiveContextPressure -count=1 -timeout 30s
go test ./internal/mcp -run TestContextPressure -count=1 -timeout 30s
```

---

#### Agent C -- Cost Velocity MCP Tool

**Goal:** Add a `get_cost_velocity` MCP tool that reports cost per minute in a
rolling window for the active session.

**Context:** claudewatch is a Go CLI. The existing `ParseLiveTokenWindow` in
`internal/claude/active_live.go` shows the exact pattern: read JSONL, filter
entries by time window, sum tokens. The `analyzer` package has `DefaultPricing`
with per-million-token rates. To avoid a circular import (`claude` cannot
import `analyzer`), define a simple `CostPricing` struct in the new file.

**Files you own (create):**
- `internal/claude/active_live_cost.go` (new)
- `internal/claude/active_live_cost_test.go` (new)
- `internal/mcp/cost_velocity_tools.go` (new)
- `internal/mcp/cost_velocity_tools_test.go` (new)

**Do not modify any other files.**

**Requirements:**

1. Create `internal/claude/active_live_cost.go`:
   - Define `CostPricing` struct with `InputPerMillion` and `OutputPerMillion` float64 fields.
   - Define `LiveCostVelocityStats` struct (see Interface Contracts).
   - Implement `ParseLiveCostVelocity(path string, windowMinutes float64, pricing CostPricing) (*LiveCostVelocityStats, error)`:
     a. Call `readLiveJSONL(path)`.
     b. Filter assistant entries to those within the last `windowMinutes`.
     c. Sum input and output tokens in the window.
     d. Compute `WindowCostUSD = (inputTokens/1M * pricing.InputPerMillion) + (outputTokens/1M * pricing.OutputPerMillion)`.
     e. Compute `CostPerMinute = WindowCostUSD / windowMinutes`.
     f. Compute status:
        - `CostPerMinute < 0.05` -> "efficient"
        - `0.05 - 0.20` -> "normal"
        - `>= 0.20` -> "burning"

2. Create `internal/mcp/cost_velocity_tools.go`:
   - Define `CostVelocityResult` struct (see Interface Contracts).
   - Implement `addCostVelocityTools(s *Server)` that registers `get_cost_velocity`.
   - Handler: find active session, call `ParseLiveCostVelocity` with 10-minute
     window and sonnet pricing (`analyzer.DefaultPricing["sonnet"]` -- convert
     to `claude.CostPricing`).
   - Description: "Cost per minute in a rolling 10-minute window for the active
     session. Returns window cost, cost/minute rate, and status (efficient/
     normal/burning)."

3. Write tests:
   - `active_live_cost_test.go`: test `ParseLiveCostVelocity` with mock JSONL.
     Need assistant entries with usage data and timestamps within/outside the
     window. Test: empty file, all outside window, entries within window,
     status thresholds.
   - `cost_velocity_tools_test.go`: test MCP handler integration.

**Interface contracts consumed:**
```go
// internal/claude/active_live.go (existing, do not modify)
func readLiveJSONL(path string) ([]TranscriptEntry, error)  // unexported, same package
func ParseTimestamp is in transcripts.go, same package

// internal/analyzer/cost.go (consumed by MCP layer only, not claude package)
var DefaultPricing map[string]ModelPricing
```

**Verification gate:**
```bash
go build ./...
go vet ./...
go test ./internal/claude -run TestParseLiveCostVelocity -count=1 -timeout 30s
go test ./internal/mcp -run TestCostVelocity -count=1 -timeout 30s
```

---

#### Agent D -- Friction Pattern Classification

**Goal:** Enhance `ParseLiveFriction` to collapse repeated friction events
into grouped patterns. Add a `Patterns` field alongside the existing `Events`
field in both `LiveFrictionStats` and `LiveFrictionResult`.

**Context:** The existing `ParseLiveFriction` in `internal/claude/active_live.go`
produces individual `LiveFrictionEvent` entries. The MCP handler in
`internal/mcp/live_tools.go` returns them via `LiveFrictionResult`. The
enhancement adds a post-processing step that groups events into patterns like
"tool_error:Bash x 3 (burst at turn 42-44)".

**Files you own (modify):**
- `internal/claude/active_live.go` (modify)
- `internal/claude/active_live_test.go` (modify -- add new tests)
- `internal/mcp/live_tools.go` (modify)
- `internal/mcp/live_tools_test.go` (modify -- add new tests)

**Do not modify any other files.**

**Requirements:**

1. Add to `internal/claude/active_live.go`:
   - Define `FrictionPattern` struct:
     ```go
     type FrictionPattern struct {
         Type        string `json:"type"`
         Count       int    `json:"count"`
         Consecutive bool   `json:"consecutive"`
         FirstTurn   int    `json:"first_turn"`
         LastTurn    int    `json:"last_turn"`
     }
     ```
   - Add `Patterns []FrictionPattern` field to `LiveFrictionStats`.
   - Add a `collapseFrictionPatterns(events []LiveFrictionEvent) []FrictionPattern`
     function that:
     a. Groups events by a key: `"{type}:{tool}"` (e.g. `"tool_error:Edit"`,
        `"retry:Bash"`, `"error_burst:"`).
     b. For each group, counts occurrences and tracks whether they were
        consecutive (no other event type between them).
     c. Tracks turn numbers: assign a sequential turn number to each event
        (position in the events slice) and record first/last.
     d. Returns sorted by count descending.
   - Call `collapseFrictionPatterns` at the end of `ParseLiveFriction` before
     returning, and assign result to `stats.Patterns`.

2. Modify `internal/mcp/live_tools.go`:
   - Add `Patterns []claude.FrictionPattern` field to `LiveFrictionResult`
     with JSON tag `"patterns,omitempty"`.
   - In `handleGetLiveFriction`, pass `stats.Patterns` through to the result.

3. Add tests:
   - In `active_live_test.go`: test `collapseFrictionPatterns` directly and
     test that `ParseLiveFriction` populates the `Patterns` field. Test cases:
     - No events -> empty patterns
     - Multiple tool_error events for same tool -> single pattern with count
     - Mixed events -> multiple patterns sorted by count
     - Consecutive detection: 3 consecutive Edit errors -> `Consecutive: true`
   - In `live_tools_test.go`: verify that `LiveFrictionResult.Patterns` is
     populated in the MCP response.

**Important:** Do not break existing tests. The `Events` field and
`TotalFriction` must continue to work exactly as before. `Patterns` is
purely additive.

**Verification gate:**
```bash
go build ./...
go vet ./...
go test ./internal/claude -run TestParseLiveFriction -count=1 -timeout 30s
go test ./internal/mcp -run TestLiveFriction -count=1 -timeout 30s
```

---

### Wave Execution Loop

After Wave 1 completes (all 4 agents):

1. Read each agent's completion report.
2. Merge all agent worktrees into main branch.
3. **Post-merge integration steps** (orchestrator):
   a. In `internal/mcp/tools.go`, add to the `addTools` function:
      ```go
      addContextTools(s)
      addCostVelocityTools(s)
      ```
      (Note: `addLiveTools(s)` already exists and handles the modified
      friction tool -- no change needed there.)
   b. In `internal/app/root.go`, register the hook subcommand:
      ```go
      rootCmd.AddCommand(hookCmd())
      ```
   c. Run `gofmt -w .` to normalize formatting.
4. Run full verification:
   ```bash
   go build ./...
   go vet ./...
   go test ./... -count=1 -timeout 120s
   ```
5. Fix any integration issues (e.g., test count assertions in `jsonrpc_test.go`
   that check tool list length).
6. Commit the wave's changes.

**Cascade candidates to verify post-merge:**
- `internal/mcp/jsonrpc_test.go` -- may assert a specific tool count in
  `tools/list` response. Update expected count from 19 to 21.
- `internal/mcp/tools_test.go` -- may test tool registration count.

---

### Status

- [SKIPPED] Wave 1 Agent A -- Friction alert hook (deferred — build hook separately after validating push model)
- [x] Wave 1 Agent B -- `get_context_pressure` MCP tool
- [ ] Wave 1 Agent C -- `get_cost_velocity` MCP tool
- [ ] Wave 1 Agent D -- Friction pattern classification
- [ ] Post-merge -- Register tools in `tools.go`, fix test counts

---

### Agent B -- Completion Report

**Status:** COMPLETE

**Files created:**
- `internal/claude/active_live_context.go` -- `ContextPressureStats` struct and `ParseLiveContextPressure` function
- `internal/claude/active_live_context_test.go` -- 8 test cases including threshold boundary tests
- `internal/mcp/context_tools.go` -- `ContextPressureResult` struct, `addContextTools`, and `handleGetContextPressure`
- `internal/mcp/context_tools_test.go` -- 5 MCP integration tests

**Implementation notes:**
- `ParseLiveContextPressure` reads JSONL via `readLiveJSONL`, sums all assistant message token usage, counts `"summary"` entries as compactions, and uses the last assistant message's `input_tokens` as the context utilization proxy (divided by 200K context window).
- Status thresholds: comfortable (<0.5), filling (0.5-0.75), pressure (0.75-0.9), critical (>=0.9).
- MCP handler follows the exact pattern from `velocity_tools.go`: find active session, parse meta, parse context pressure, return typed result.

**Verification:**
- `go build ./...` -- pass
- `go vet ./...` -- pass
- `go test ./internal/claude -run TestParseLiveContextPressure` -- 8/8 pass
- `go test ./internal/mcp -run TestContextPressure` -- 5/5 pass

**Post-merge required:** Add `addContextTools(s)` call in `internal/mcp/tools.go` `addTools` function.
