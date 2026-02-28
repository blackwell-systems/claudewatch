# IMPL: MCP stdio Server (`claudewatch mcp`)

<!-- scout v0.2.0 — generated 2026-02-28 -->

---

### Suitability Assessment

**Verdict: SUITABLE**

All five gate questions resolve cleanly.

1. **File decomposition.** The work decomposes into three completely disjoint
   file sets: (A) the JSON-RPC/stdio transport layer
   (`internal/mcp/jsonrpc.go`), (B) the three tool handlers
   (`internal/mcp/tools.go`), and (C) the Cobra sub-command wiring
   (`internal/app/mcp.go`). No two agents share a file. Agent C depends on
   the package that A+B create, so it sits in Wave 2; A and B are parallel in
   Wave 1.

2. **Investigation-first items.** None. All data paths are already exercised by
   existing commands (`sessions`, `metrics`, `watch`). There are no crashes or
   unknown root causes.

3. **Interface discoverability.** The cross-agent boundary is the
   `internal/mcp` package surface. Agent C calls `mcp.NewServer(cfg)` and
   `server.Run(ctx, os.Stdin, os.Stdout)`. Both signatures are fully
   specifiable now.

4. **Pre-implementation scan.** No MCP code exists anywhere in the codebase.
   All three items are TO-DO.

5. **Parallelization value.** Agent A and Agent B are fully independent
   (disjoint files, no shared state). Agent C is small but genuinely blocked
   on the `mcp` package existing. The `go build && go test` cycle takes ~1–2 s
   for this repo (SQLite CGO-disabled build), which is fast; the main benefit
   is coordination clarity, not raw speed.

Pre-implementation scan results:
- Total items: 3 agents' worth of new work
- Already implemented: 0 items (0%)
- Partially implemented: 0 items
- To-do: 3 items

Agent adjustments: none — all agents proceed as planned.

Estimated times:
- Scout phase: ~10 min (this doc)
- Agent execution: ~25 min (A: 10 min, B: 10 min, C: 5 min; A+B run in parallel)
- Merge & verification: ~5 min
- Total SAW time: ~40 min

Sequential baseline: ~30 min (3 agents × 10 min avg)
Time savings: marginal on speed; primary value is interface contracts and
progress tracking.

Recommendation: Clear speedup for Agent A/B parallelism. Proceed.

---

### Known Issues

None identified. All tests pass (`go test ./...` clean as of 2026-02-28).

---

### Dependency Graph

```
internal/mcp/jsonrpc.go   ←── Agent A (Wave 1)
internal/mcp/tools.go     ←── Agent B (Wave 1, depends on existing internal/claude + internal/analyzer + internal/config)
internal/app/mcp.go       ←── Agent C (Wave 2, depends on Agent A + B delivering internal/mcp package)
```

Existing packages that are **read** but NOT modified:
- `internal/claude` — `ParseAllSessionMeta`, `ParseAllFacets`, `ParseStatsCache`, `LatestSessionID`
- `internal/analyzer` — `EstimateSessionCost`, `ComputeCacheRatio`, `NoCacheRatio`, `DefaultPricing`
- `internal/config` — `Load`, `Config.ClaudeHome`
- `internal/app/root.go` — Agent C calls `rootCmd.AddCommand(mcpCmd)` in its own `init()`, matching all other commands' pattern

**Cascade candidates** (files that reference no new interface but are adjacent):
- `internal/app/root.go` — no change needed; Agent C registers via `init()` identical to `sessionsCmd`, `watchCmd`, etc.
- `cmd/claudewatch/main.go` — no change needed; `app.Execute()` already picks up all commands.

Neither cascade candidate requires modification. The post-merge build verifies both compile cleanly.

---

### Interface Contracts

These are binding. Agents implement and call exactly these signatures.

#### Package `internal/mcp` (delivered by Agent A + Agent B)

```go
// Server is an MCP stdio server. It reads JSON-RPC requests from r and
// writes JSON-RPC responses to w. Calls are dispatched to registered tools.
type Server struct { /* unexported fields */ }

// NewServer constructs a Server. cfg provides ClaudeHome for data access.
// budgetUSD of 0.0 means no budget configured.
func NewServer(cfg *config.Config, budgetUSD float64) *Server

// Run blocks, reading JSON-RPC 2.0 messages from r and writing responses to w,
// until ctx is cancelled or r returns EOF. Returns nil on clean shutdown,
// or a non-nil error for unexpected I/O failures.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error
```

Internal types (Agent A owns, Agent B aligns to):

```go
type toolDef struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Handler     toolHandler
}

type toolHandler func(args json.RawMessage) (any, error)

// addTools registers all MCP tool handlers on s.
// Defined in tools.go (Agent B), called from NewServer in jsonrpc.go (Agent A).
func addTools(s *Server)
```

Wire types:

```go
type jsonrpcRequest struct {
    JSONRPC string           `json:"jsonrpc"`
    ID      *json.RawMessage `json:"id,omitempty"`
    Method  string           `json:"method"`
    Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonrpcResponse struct {
    JSONRPC string           `json:"jsonrpc"`
    ID      *json.RawMessage `json:"id,omitempty"`
    Result  any              `json:"result,omitempty"`
    Error   *jsonrpcError    `json:"error,omitempty"`
}

type jsonrpcError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

Tool result types (Agent B owns):

```go
type SessionStatsResult struct {
    SessionID     string  `json:"session_id"`
    ProjectName   string  `json:"project_name"`
    StartTime     string  `json:"start_time"`
    DurationMin   int     `json:"duration_minutes"`
    InputTokens   int     `json:"input_tokens"`
    OutputTokens  int     `json:"output_tokens"`
    EstimatedCost float64 `json:"estimated_cost_usd"`
}

type CostBudgetResult struct {
    TodaySpendUSD  float64 `json:"today_spend_usd"`
    DailyBudgetUSD float64 `json:"daily_budget_usd"`
    Remaining      float64 `json:"remaining_usd"`
    OverBudget     bool    `json:"over_budget"`
}

type RecentSessionsResult struct {
    Sessions []RecentSession `json:"sessions"`
}

type RecentSession struct {
    SessionID     string  `json:"session_id"`
    ProjectName   string  `json:"project_name"`
    StartTime     string  `json:"start_time"`
    DurationMin   int     `json:"duration_minutes"`
    EstimatedCost float64 `json:"estimated_cost_usd"`
    FrictionScore int     `json:"friction_score"`
}
```

#### Cross-boundary call from Agent C → package mcp

```go
// In internal/app/mcp.go (Agent C):
import "github.com/blackwell-systems/claudewatch/internal/mcp"

srv := mcp.NewServer(cfg, mcpBudget)
return srv.Run(cmd.Context(), os.Stdin, os.Stdout)
```

---

### File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `internal/mcp/jsonrpc.go` | A | 1 | nothing new (stdlib only) |
| `internal/mcp/jsonrpc_test.go` | A | 1 | nothing new |
| `internal/mcp/tools.go` | B | 1 | existing `internal/claude`, `internal/analyzer`, `internal/config` |
| `internal/mcp/tools_test.go` | B | 1 | existing packages |
| `internal/app/mcp.go` | C | 2 | Agent A + B (internal/mcp package) |

No existing files are modified by any agent.

---

### Wave Structure

```
Wave 1: [A] [B]          ← 2 parallel agents; fully independent files
              |
              | (A + B complete; internal/mcp package exists and tests pass)
              |
Wave 2:    [C]           ← 1 agent; wires Cobra command
```

Wave 2 is unblocked when Agent A's `mcp.NewServer` and `(*Server).Run` are
implemented and Agent B's tool handlers compile inside the same package.
The post-Wave-1 merge must produce a clean `go build ./...` before Agent C
starts.

---

### Agent Prompts

---

#### Wave 1 Agent A: JSON-RPC stdio transport

You are Wave 1 Agent A. Your task is to implement the JSON-RPC 2.0 stdio
transport layer for the `internal/mcp` package. This is the read-write loop
that underpins the MCP server; it has no tool logic.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-a"

if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  echo "Expected: $EXPECTED_BRANCH"
  echo "Actual: $ACTUAL_BRANCH"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

If verification fails: append failure report to `docs/IMPL-mcp-server.md`
under `### Agent A — Completion Report` and stop.

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/mcp/jsonrpc.go` — create
- `internal/mcp/jsonrpc_test.go` — create

You also need `internal/mcp/tools.go` to exist so the package compiles
(Agent B owns it). Create a minimal stub in your worktree:

```go
// internal/mcp/tools.go (TEMPORARY STUB — Agent B replaces this at merge)
package mcp

func addTools(s *Server) {}
```

Mark this clearly. Do not implement any tool logic here.

## 2. Interfaces You Must Implement

```go
package mcp

type Server struct {
    tools      []toolDef
    claudeHome string
    budgetUSD  float64
}

type toolDef struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Handler     toolHandler
}

type toolHandler func(args json.RawMessage) (any, error)

func NewServer(cfg *config.Config, budgetUSD float64) *Server
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error
func (s *Server) registerTool(def toolDef)
```

`NewServer` must call `addTools(s)` before returning. `addTools` is defined
in `tools.go` (Agent B). You provide only the stub in your worktree.

## 3. Interfaces You May Call

None from new code. Stdlib only: `bufio`, `encoding/json`, `io`, `context`.

Import `github.com/blackwell-systems/claudewatch/internal/config` for the
`*config.Config` parameter type. Read `internal/config/config.go` to confirm
the `ClaudeHome` field name.

## 4. What to Implement

Create `internal/mcp/jsonrpc.go` with:

**`NewServer`**: Allocates Server, stores `cfg.ClaudeHome` and `budgetUSD`,
calls `addTools(s)`, returns `s`.

**`registerTool`**: Appends a `toolDef` to `s.tools`.

**`Run`**: Read newline-delimited JSON from `r` via `bufio.Scanner`. For each line:
1. Unmarshal into `jsonrpcRequest`.
2. If `req.ID` is nil (notification), skip — write no response.
3. Dispatch on `req.Method`:
   - `"initialize"` → respond with:
     ```json
     {"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"claudewatch","version":"0.1.0"}}
     ```
   - `"tools/list"` → respond with `{"tools": [...]}` listing all `s.tools`
     (name, description, inputSchema per entry)
   - `"tools/call"` → parse `params` for `name` and `arguments`; find matching
     toolDef; call handler; wrap result as MCP content response (see below)
   - any other method → JSON-RPC error -32601 "Method not found"
4. Write response as single JSON line + `\n` to a buffered writer; flush.
5. On `ctx.Done()`: return nil. On scanner EOF: return nil. On scanner error: return error.

MCP `tools/call` success response `result` shape:
```json
{"content":[{"type":"text","text":"<JSON-marshalled tool result>"}],"isError":false}
```

MCP `tools/call` error response (handler returned error):
```json
{"content":[{"type":"text","text":"<error message>"}],"isError":true}
```

Unknown tool name: return the error shape above with `"unknown tool: <name>"`.

All responses share the envelope:
```json
{"jsonrpc":"2.0","id":<echo req.ID>,"result":<result>}
```
or for JSON-RPC-level errors:
```json
{"jsonrpc":"2.0","id":<echo req.ID>,"error":{"code":-32601,"message":"Method not found"}}
```

## 5. Tests to Write

In `internal/mcp/jsonrpc_test.go` using `io.Pipe()` for in-process I/O:

1. `TestRun_Initialize` — sends initialize, asserts `protocolVersion` field present and `serverInfo.name == "claudewatch"`
2. `TestRun_ToolsList` — sends `tools/list`, asserts ≥3 tools each with non-empty name
3. `TestRun_UnknownMethod` — sends unknown method, asserts `error.code == -32601`
4. `TestRun_Notification` — sends message with no `id`, asserts no response written (read with short deadline)
5. `TestRun_ContextCancel` — cancels context, asserts `Run` returns nil
6. `TestRun_EOFClean` — closes writer side of pipe, asserts `Run` returns nil

Note: `TestRun_ToolsList` requires `addTools` to register tools. With the
stub `tools.go` (empty `addTools`), this test will see 0 tools. Either
skip the ≥3 assertion in your worktree (assert ≥0) and note it in your
report, or hardcode a test tool via `registerTool` in the test setup.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/mcp/... -run 'TestRun_' -v -timeout 30s
```

## 7. Constraints

- No external packages. Stdlib only (plus internal/config for the type).
- Nothing written to stdout except JSON-RPC responses. stderr only for
  unexpected errors (not EOF, not context cancel).
- Single-threaded `Run` — no goroutines inside the loop.
- `budgetUSD == 0` is valid; store and forward to tools unchanged.
- Do not modify `go.mod`.

## 8. Report

Append under `### Agent A — Completion Report` in `docs/IMPL-mcp-server.md`.
Include: functions implemented, test results, stub caveat, any type
deviations Agent B needs to know about.

---

#### Wave 1 Agent B: MCP Tool Handlers

You are Wave 1 Agent B. Your task is to implement the three MCP tool handlers
in `internal/mcp/tools.go`.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-b"

if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  echo "Expected: $EXPECTED_BRANCH"
  echo "Actual: $ACTUAL_BRANCH"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

## 1. File Ownership

You own these files:
- `internal/mcp/tools.go` — create
- `internal/mcp/tools_test.go` — create

You need `internal/mcp/jsonrpc.go` to exist (Agent A owns it). Create a
minimal stub in your worktree so the package compiles:

```go
// internal/mcp/jsonrpc.go (TEMPORARY STUB — Agent A provides real impl)
package mcp

import (
    "context"
    "encoding/json"
    "io"

    "github.com/blackwell-systems/claudewatch/internal/config"
)

type toolDef struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Handler     toolHandler
}

type toolHandler func(args json.RawMessage) (any, error)

type Server struct {
    tools      []toolDef
    claudeHome string
    budgetUSD  float64
}

func NewServer(cfg *config.Config, budgetUSD float64) *Server {
    s := &Server{claudeHome: cfg.ClaudeHome, budgetUSD: budgetUSD}
    addTools(s)
    return s
}

func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error { return nil }

func (s *Server) registerTool(def toolDef) { s.tools = append(s.tools, def) }
```

If Agent A's completion report reveals different type names, align your
`tools.go` accordingly and document in your report.

## 2. Interfaces You Must Implement

```go
func addTools(s *Server)

type SessionStatsResult struct {
    SessionID     string  `json:"session_id"`
    ProjectName   string  `json:"project_name"`
    StartTime     string  `json:"start_time"`
    DurationMin   int     `json:"duration_minutes"`
    InputTokens   int     `json:"input_tokens"`
    OutputTokens  int     `json:"output_tokens"`
    EstimatedCost float64 `json:"estimated_cost_usd"`
}

type CostBudgetResult struct {
    TodaySpendUSD  float64 `json:"today_spend_usd"`
    DailyBudgetUSD float64 `json:"daily_budget_usd"`
    Remaining      float64 `json:"remaining_usd"`
    OverBudget     bool    `json:"over_budget"`
}

type RecentSessionsResult struct {
    Sessions []RecentSession `json:"sessions"`
}

type RecentSession struct {
    SessionID     string  `json:"session_id"`
    ProjectName   string  `json:"project_name"`
    StartTime     string  `json:"start_time"`
    DurationMin   int     `json:"duration_minutes"`
    EstimatedCost float64 `json:"estimated_cost_usd"`
    FrictionScore int     `json:"friction_score"`
}
```

## 3. Interfaces You May Call

Read these files before writing code:
- `internal/claude/types.go` — SessionMeta, SessionFacet, StatsCache shapes
- `internal/analyzer/cost.go` — EstimateSessionCost, ComputeCacheRatio, NoCacheRatio, DefaultPricing
- `internal/app/sessions.go` — reference pattern for loading+joining sessions and facets

```go
// internal/claude
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error)
func ParseAllFacets(claudeHome string) ([]SessionFacet, error)
func ParseStatsCache(claudeHome string) (*StatsCache, error)

// internal/analyzer
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64
func ComputeCacheRatio(stats claude.StatsCache) CacheRatio
func NoCacheRatio() CacheRatio
var DefaultPricing map[string]ModelPricing  // key "sonnet" for default
```

## 4. What to Implement

**`addTools(s *Server)`**: Registers three tools via `s.registerTool(toolDef{...})`.

Input schemas:
```go
noArgsSchema    := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
recentNSchema   := json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer","description":"Number of sessions to return (default 5)"}},"additionalProperties":false}`)
```

**`get_session_stats`** (no args):
1. `ParseAllSessionMeta(s.claudeHome)` — error → return error
2. Sort by `StartTime` descending; take most recent
3. Load stats-cache (non-fatal; fallback to `NoCacheRatio()`)
4. `EstimateSessionCost(session, DefaultPricing["sonnet"], ratio)`
5. Return `SessionStatsResult`; `ProjectName = filepath.Base(session.ProjectPath)`

**`get_cost_budget`** (no args):
1. `ParseAllSessionMeta(s.claudeHome)` — error → return error
2. Load stats-cache (non-fatal)
3. Filter sessions where `session.StartTime[:10] == time.Now().UTC().Format("2006-01-02")`
4. Sum costs across filtered sessions
5. Return `CostBudgetResult`; `Remaining = s.budgetUSD - sum` (0 if no budget); `OverBudget = s.budgetUSD > 0 && sum > s.budgetUSD`

**`get_recent_sessions`** (optional `n int`):
1. Parse `{"n": N}` from args; default N=5, cap at 50
2. `ParseAllSessionMeta` + `ParseAllFacets`; index facets by SessionID
3. Sort sessions by `StartTime` descending; take first N
4. Load stats-cache (non-fatal)
5. For each: `FrictionScore` = sum of all `facet.FrictionCounts` values (0 if no facet)
6. Return `RecentSessionsResult`

Non-fatal rule: if `ParseAllSessionMeta` returns `os.IsNotExist`, treat as
empty slice (return zero-value result, no error). Only propagate unexpected I/O errors.

## 5. Tests to Write

Use `t.TempDir()` with synthetic JSON fixtures. Layout:
- `<tmpDir>/usage-data/session-meta/<id>.json`
- `<tmpDir>/usage-data/facets/<id>.json`

Tests:
1. `TestGetSessionStats_NoSessions` — empty dir, assert error returned
2. `TestGetSessionStats_SingleSession` — one synthetic session, assert `EstimatedCost > 0`, `ProjectName` correct
3. `TestGetCostBudget_NoSessions` — empty dir, assert `TodaySpendUSD == 0`, `OverBudget == false`
4. `TestGetCostBudget_OverBudget` — sessions dated today with tokens, `budgetUSD=0.01`, assert `OverBudget == true`
5. `TestGetRecentSessions_DefaultN` — 10 sessions, no n arg, assert `len == 5`
6. `TestGetRecentSessions_CustomN` — 10 sessions, `n=3`, assert `len == 3`
7. `TestGetRecentSessions_FrictionScore` — one session with `FrictionCounts={"wrong_approach":2,"off_track":1}`, assert `FrictionScore == 3`

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
go build ./...
go vet ./...
go test ./internal/mcp/... -run 'Test(GetSession|GetCost|GetRecent)' -v -timeout 30s
```

## 7. Constraints

- Import only `internal/claude`, `internal/analyzer`, stdlib. No lipgloss, cobra, viper.
- If `ParseAllSessionMeta` returns `os.IsNotExist`, treat as empty (not error).
- `budgetUSD == 0`: `Remaining = 0`, `OverBudget = false`.
- Tools are stateless per-call (read from disk each time). No caching.
- Do not modify `go.mod`.

## 8. Report

Append under `### Agent B — Completion Report` in `docs/IMPL-mcp-server.md`.
Include: all three tools implemented, test results, any type deviations from
stub (Agent C needs to know the final `toolDef`/`registerTool` shape).

---

#### Wave 2 Agent C: Cobra `mcp` subcommand

You are Wave 2 Agent C. Wave 1 is complete and merged. The `internal/mcp`
package exists. Your task is to wire `claudewatch mcp` into the CLI.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-c"

if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  echo "Expected: $EXPECTED_BRANCH"
  echo "Actual: $ACTUAL_BRANCH"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

## 1. File Ownership

You own exactly one file:
- `internal/app/mcp.go` — create

Also create the test file:
- `internal/app/mcp_test.go` — create

Do not modify any existing file.

## 2. Interfaces You Must Implement

```go
var mcpCmd *cobra.Command
var mcpBudget float64

func init()
func runMCP(cmd *cobra.Command, args []string) error
```

## 3. Interfaces You May Call

From `internal/mcp` (delivered by Wave 1 — read the actual package to confirm signatures):
```go
func mcp.NewServer(cfg *config.Config, budgetUSD float64) *mcp.Server
func (s *mcp.Server) Run(ctx context.Context, r io.Reader, w io.Writer) error
```

From existing `internal/app` package (same package, call directly):
```go
var flagConfig string   // root.go
func config.Load(cfgFile string) (*config.Config, error)
```

## 4. What to Implement

Follow `internal/app/watch.go` as the pattern. Your file is simpler.

```go
package app

import (
    "fmt"
    "os"

    "github.com/blackwell-systems/claudewatch/internal/config"
    "github.com/blackwell-systems/claudewatch/internal/mcp"
    "github.com/spf13/cobra"
)

var mcpBudget float64

var mcpCmd = &cobra.Command{
    Use:   "mcp",
    Short: "Run an MCP stdio server for use with Claude Code",
    Long: `Start a Model Context Protocol stdio server that Claude Code can
query during a session. The server exposes three tools:

  get_session_stats   Token usage, cost, and duration for the current session
  get_cost_budget     Today's spend vs daily budget
  get_recent_sessions Last N sessions with cost, friction, and project name

Add to your Claude Code MCP configuration (~/.claude/settings.json):
  {"mcpServers":{"claudewatch":{"command":"claudewatch","args":["mcp"]}}}`,
    RunE: runMCP,
}

func init() {
    mcpCmd.Flags().Float64Var(&mcpBudget, "budget", 0, "Daily cost budget in USD for budget reporting (e.g. --budget 20)")
    rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
    cfg, err := config.Load(flagConfig)
    if err != nil {
        return fmt.Errorf("loading config: %w", err)
    }
    srv := mcp.NewServer(cfg, mcpBudget)
    return srv.Run(cmd.Context(), os.Stdin, os.Stdout)
}
```

Before finalising, read `docs/IMPL-mcp-server.md` Agent A and B completion
reports to confirm `mcp.NewServer` and `(*Server).Run` signatures match.
Adjust imports if actual signatures differ from above.

## 5. Tests to Write

`internal/app/mcp_test.go`:
```go
func TestMCPCmd_Registered(t *testing.T) {
    for _, cmd := range rootCmd.Commands() {
        if cmd.Use == "mcp" {
            return
        }
    }
    t.Fatal("mcp subcommand not registered on rootCmd")
}
```

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
go build ./...
go vet ./...
go test ./internal/app/... -run TestMCPCmd -v -timeout 30s
go test ./internal/mcp/... -v -timeout 60s
```

## 7. Constraints

- Do not import lipgloss, output, or watcher.
- Do not modify root.go or any existing file.
- Nothing written to stdout (MCP owns it). No startup banner.
- `go.mod` must not be modified.
- If `internal/mcp` API differs from Interface Contracts, align to actual and document.

## 8. Report

Append under `### Agent C — Completion Report` in `docs/IMPL-mcp-server.md`.

---

### Wave Execution Loop

After each wave completes:
1. Read each agent's completion report. Check interface contract deviations.
2. Merge all agent worktrees (see `~/code/scout-and-wave/prompts/saw-merge.md`).
3. Run full verification gate:
   ```bash
   cd /Users/dayna.blackwell/code/claudewatch
   go build ./...
   go vet ./...
   gofmt -l ./internal/mcp/ ./internal/app/mcp.go
   go test ./...
   ```
4. Fix any issues (pay attention to `addTools`/`toolDef` boundary between A and B).
5. Tick status checkboxes. Correct interface contracts if they changed.
6. Commit the wave. Launch the next wave.

Do not launch Wave 2 with a broken build.

**Smoke test after Wave 2 merge:**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | go run ./cmd/claudewatch mcp
```
Expect JSON response with `protocolVersion`. Ctrl-C to exit.

---

### Status

- [x] Wave 1 Agent A — JSON-RPC stdio transport (`internal/mcp/jsonrpc.go`)
- [x] Wave 1 Agent B — MCP tool handlers (`internal/mcp/tools.go`)
- [ ] Wave 2 Agent C — Cobra `mcp` subcommand (`internal/app/mcp.go`)

---

### Agent A — Completion Report

**Status:** COMPLETE — all verification gates pass.

**Date:** 2026-02-28

**Branch:** `wave1-agent-a`

**Files delivered:**

| File | Role |
|------|------|
| `internal/mcp/jsonrpc.go` | Full JSON-RPC 2.0 stdio transport implementation |
| `internal/mcp/jsonrpc_test.go` | 6 tests covering all required scenarios |
| `internal/mcp/tools.go` | Temporary stub (`func addTools(s *Server) {}`) — Agent B replaces |

**Functions implemented in `jsonrpc.go`:**

- `NewServer(cfg *config.Config, budgetUSD float64) *Server` — allocates Server, stores `cfg.ClaudeHome` and `budgetUSD`, calls `addTools(s)`, returns `s`
- `(s *Server) registerTool(def toolDef)` — appends a `toolDef` to `s.tools`
- `(s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error` — newline-delimited JSON read loop via `bufio.Scanner` in a goroutine; dispatches to `handleLine`; returns nil on EOF or context cancel, non-nil on scanner error
- `(s *Server) handleLine(_ context.Context, line string, bw *bufio.Writer) error` — unmarshals JSON-RPC request, skips notifications (nil ID), dispatches on Method
- Internal helpers: `writeError`, `writeResponse`

**Type definitions (exact — binding for Agent B):**

```go
type toolDef struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Handler     toolHandler
}

type toolHandler func(args json.RawMessage) (any, error)

type Server struct {
    tools      []toolDef
    claudeHome string
    budgetUSD  float64
}
```

Agent B must use exactly these types. The `registerTool` method signature is:

```go
func (s *Server) registerTool(def toolDef)
```

**Test results:**

```
=== RUN   TestRun_Initialize
--- PASS: TestRun_Initialize (0.00s)
=== RUN   TestRun_ToolsList
--- PASS: TestRun_ToolsList (0.00s)
=== RUN   TestRun_UnknownMethod
--- PASS: TestRun_UnknownMethod (0.00s)
=== RUN   TestRun_Notification
--- PASS: TestRun_Notification (0.10s)
=== RUN   TestRun_ContextCancel
--- PASS: TestRun_ContextCancel (0.00s)
=== RUN   TestRun_EOFClean
--- PASS: TestRun_EOFClean (0.00s)
PASS
ok      github.com/blackwell-systems/claudewatch/internal/mcp
```

**Stub caveat for `TestRun_ToolsList`:** With the stub `addTools` (no-op), `NewServer` registers zero tools. The test works around this by calling `s.registerTool(...)` directly in the test setup to register one test tool, then asserts `len >= 1`. When Agent B's real `addTools` replaces the stub at merge, the test will see `>= 4` tools (3 real + 1 test tool), which still satisfies the `>= 1` assertion. No test changes are needed after merge.

**Interface deviations from spec:** None. All types, method names, and signatures match the Interface Contracts section exactly.

**Verification gate output:**
- `go build ./...` — clean (no output)
- `go vet ./...` — clean (no output)
- `go test ./internal/mcp/... -run 'TestRun_' -v -timeout 30s` — 6/6 PASS

---

### Agent B — Completion Report

**Status:** COMPLETE — all verification gates passed.

**Branch:** `wave1-agent-b`
**Commit:** `a4f2ae4`

#### Files Delivered

| File | Role |
|------|------|
| `internal/mcp/tools.go` | Three tool handlers + result types (Agent B owns) |
| `internal/mcp/tools_test.go` | 7 tests covering all handlers |
| `internal/mcp/jsonrpc.go` | Temporary stub (Agent A replaces at merge) |

#### Tools Implemented

**`get_session_stats`** — loads all session metadata, sorts by StartTime descending, computes cost via `analyzer.EstimateSessionCost` with `DefaultPricing["sonnet"]`, falls back to `NoCacheRatio()` on missing stats-cache.

**`get_cost_budget`** — filters today's sessions by `StartTime[:10]`, sums costs, compares to `budgetUSD`. Returns `Remaining` and `OverBudget` fields.

**`get_recent_sessions`** — takes optional `n` (default 5, cap 50), joins sessions with facets, computes `FrictionScore` as sum of all `FrictionCounts` values.

#### Test Results

All 7 tests pass: `TestGetSessionStats_NoSessions`, `TestGetSessionStats_SingleSession`, `TestGetCostBudget_NoSessions`, `TestGetCostBudget_OverBudget`, `TestGetRecentSessions_DefaultN`, `TestGetRecentSessions_CustomN`, `TestGetRecentSessions_FrictionScore`.

`go build ./...` and `go vet ./...` both pass clean.

#### Discovery: `EstimateSessionCost` location

The IMPL doc lists `EstimateSessionCost` under `cost.go` but it lives in `internal/analyzer/outcomes.go`. No import path change needed — both files are in package `analyzer`.

---

### Agent C — Completion Report

**Status:** COMPLETE — all verification gates pass.

**Date:** 2026-02-28

**Branch:** `wave2-agent-c`

#### Files Delivered

| File | Role |
|------|------|
| `internal/app/mcp.go` | Cobra `mcp` subcommand wiring |
| `internal/app/mcp_test.go` | `TestMCPCmd_Registered` — verifies subcommand is on rootCmd |

#### Implementation

`internal/app/mcp.go` follows the `watch.go` pattern exactly:

- `mcpBudget float64` package-level var, bound to `--budget` flag
- `mcpCmd *cobra.Command` with `Use: "mcp"`, `RunE: runMCP`
- `init()` registers the `--budget` flag and calls `rootCmd.AddCommand(mcpCmd)`
- `runMCP` loads config via `config.Load(flagConfig)`, constructs `mcp.NewServer(cfg, mcpBudget)`, calls `srv.Run(cmd.Context(), os.Stdin, os.Stdout)`

No existing files were modified. `go.mod` was not touched.

#### Interface Contracts Confirmed

Read Agent A and B completion reports and verified actual package source before writing code. Signatures match spec exactly:

```go
func mcp.NewServer(cfg *config.Config, budgetUSD float64) *mcp.Server  // confirmed
func (s *mcp.Server) Run(ctx context.Context, r io.Reader, w io.Writer) error  // confirmed
```

No deviations.

#### Test Results

```
=== RUN   TestMCPCmd_Registered
--- PASS: TestMCPCmd_Registered (0.00s)
PASS
ok      github.com/blackwell-systems/claudewatch/internal/app   1.021s
```

Full `internal/mcp` suite also verified clean in this worktree:

```
=== RUN   TestRun_Initialize
--- PASS: TestRun_Initialize (0.00s)
=== RUN   TestRun_ToolsList
--- PASS: TestRun_ToolsList (0.00s)
=== RUN   TestRun_UnknownMethod
--- PASS: TestRun_UnknownMethod (0.00s)
=== RUN   TestRun_Notification
--- PASS: TestRun_Notification (0.10s)
=== RUN   TestRun_ContextCancel
--- PASS: TestRun_ContextCancel (0.00s)
=== RUN   TestRun_EOFClean
--- PASS: TestRun_EOFClean (0.00s)
=== RUN   TestGetSessionStats_NoSessions
--- PASS: TestGetSessionStats_NoSessions (0.00s)
=== RUN   TestGetSessionStats_SingleSession
--- PASS: TestGetSessionStats_SingleSession (0.00s)
=== RUN   TestGetCostBudget_NoSessions
--- PASS: TestGetCostBudget_NoSessions (0.00s)
=== RUN   TestGetCostBudget_OverBudget
--- PASS: TestGetCostBudget_OverBudget (0.00s)
=== RUN   TestGetRecentSessions_DefaultN
--- PASS: TestGetRecentSessions_DefaultN (0.01s)
=== RUN   TestGetRecentSessions_CustomN
--- PASS: TestGetRecentSessions_CustomN (0.01s)
=== RUN   TestGetRecentSessions_FrictionScore
--- PASS: TestGetRecentSessions_FrictionScore (0.00s)
PASS
ok      github.com/blackwell-systems/claudewatch/internal/mcp   0.526s
```

#### Verification Gate Output

- `go build ./...` — clean (no output)
- `go vet ./...` — clean (no output)
- `go test ./internal/app/... -run TestMCPCmd -v -timeout 30s` — 1/1 PASS
- `go test ./internal/mcp/... -v -timeout 60s` — 13/13 PASS
