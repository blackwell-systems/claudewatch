# IMPL: MCP Self-Model Tools

**Feature:** Five new MCP tools giving Claude real-time access to its own behavioral
patterns: `get_project_health`, `get_suggestions`, `get_agent_performance`,
`get_effectiveness`, `get_session_friction`.

---

### Suitability Assessment

Verdict: SUITABLE

All five tools are thin wrappers over existing `analyzer` and `claude` package
functions. The work decomposes cleanly: each tool (or logical group) lands in its
own new file under `internal/mcp/`, so disjoint ownership is achieved without any
agent touching `tools.go` — the registration calls in `addTools` are orchestrator-owned
and applied post-merge as append-only additions. All cross-agent interfaces are fully
specifiable from existing type signatures in `analyzer/types.go` and
`internal/suggest/types.go`. No investigation-first blockers.

Pre-implementation scan: all 5 tools are TO-DO. No overlap with existing MCP surface.

```
Estimated times:
- Scout phase: ~15 min (reading 12+ files across 4 packages)
- Wave 1: ~35 min (4 parallel agents × ~35 min avg; parallel time = max)
- Merge & verification: ~10 min
Total SAW time: ~60 min

Sequential baseline: ~150 min (4 agents × ~35 min + overhead)
Time savings: ~90 min (60% faster)

Recommendation: Clear speedup. 4 fully independent agents, each creating 2 new files.
Go test cycle ~2min amplifies parallelization benefit. Proceed.
```

---

### Known Issues

None identified. `go build ./...` and `go test ./...` pass clean on main.

---

### Dependency Graph

```
internal/claude/          — leaf data sources (no new changes)
internal/analyzer/        — leaf analysis functions (no new changes)
internal/suggest/         — leaf suggestion engine (no new changes)
internal/scanner/         — leaf project discovery (Agent A only)

internal/mcp/health_tools.go      (Agent A) — imports claude, analyzer, scanner
internal/mcp/suggest_tools.go     (Agent B) — imports claude, analyzer, suggest
internal/mcp/analytics_tools.go   (Agent C) — imports claude, analyzer
internal/mcp/friction_tools.go    (Agent D) — imports claude

internal/mcp/tools.go             (Orchestrator post-merge) — addTools registrations
```

All agents import existing packages. No new packages created. `tools.go` receives
4 `s.registerTool(...)` calls added by the orchestrator after merge, one per agent's
tool(s).

**Cascade candidates (files not in agent scope but referencing changed interfaces):**
None — all changes are additive new files. No existing types are renamed or removed.

---

### Interface Contracts

#### Orchestrator post-merge additions to `internal/mcp/tools.go`

Add to `addTools(s *Server)` after the existing registrations:

```go
// Added by orchestrator post-merge:
s.registerTool(toolDef{
    Name:        "get_project_health",
    Description: "Project-specific health metrics: friction rate, agent success rate, zero-commit rate, top error types, and whether a CLAUDE.md exists. Call at session start to calibrate behavior for the current project.",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (e.g. 'commitmux'). Omit to use the current session's project."}},"additionalProperties":false}`),
    Handler:     s.handleGetProjectHealth,
})
s.registerTool(toolDef{
    Name:        "get_suggestions",
    Description: "Ranked improvement suggestions based on session history: missing CLAUDE.md, recurring friction, low agent success rates, parallelization opportunities. Returns top N by impact score.",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Filter suggestions for a specific project name (optional)."},"limit":{"type":"integer","description":"Maximum suggestions to return (default 5, max 20)."}},"additionalProperties":false}`),
    Handler:     s.handleGetSuggestions,
})
s.registerTool(toolDef{
    Name:        "get_agent_performance",
    Description: "Agent task performance metrics across all sessions: overall success rate, kill rate, background ratio, average duration and tokens. Broken down by agent type (Explore, Plan, general-purpose, etc.).",
    InputSchema: noArgsSchema,
    Handler:     s.handleGetAgentPerformance,
})
s.registerTool(toolDef{
    Name:        "get_effectiveness",
    Description: "CLAUDE.md change effectiveness scores per project: before/after comparison of friction rate, tool errors, and goal achievement. Tells you whether your CLAUDE.md changes actually helped.",
    InputSchema: noArgsSchema,
    Handler:     s.handleGetEffectiveness,
})
s.registerTool(toolDef{
    Name:        "get_session_friction",
    Description: "Friction events recorded for a specific session. Pass the current session ID to see what friction patterns have been logged so far this session.",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to inspect. Use the current session ID from get_session_stats."}},"required":["session_id"],"additionalProperties":false}`),
    Handler:     s.handleGetSessionFriction,
})
```

#### Agent A — Result types and handler signature (`health_tools.go`)

```go
package mcp

// ProjectHealthResult holds per-project health metrics for the get_project_health tool.
type ProjectHealthResult struct {
    Project          string                       `json:"project"`
    SessionCount     int                          `json:"session_count"`
    FrictionRate     float64                      `json:"friction_rate"`
    TopFriction      []string                     `json:"top_friction_types"`
    AvgToolErrors    float64                      `json:"avg_tool_errors_per_session"`
    ZeroCommitRate   float64                      `json:"zero_commit_rate"`
    AgentSuccessRate float64                      `json:"agent_success_rate"`
    HasClaudeMD      bool                         `json:"has_claude_md"`
    ByAgentType      map[string]AgentTypeSummary  `json:"agent_performance_by_type"`
}

// AgentTypeSummary is a compact per-type agent stat for project health output.
type AgentTypeSummary struct {
    Count       int     `json:"count"`
    SuccessRate float64 `json:"success_rate"`
}

func (s *Server) handleGetProjectHealth(args json.RawMessage) (any, error)
```

Existing functions Agent A calls:
```go
claude.ParseAllSessionMeta(s.claudeHome) ([]claude.SessionMeta, error)
claude.ParseAllFacets(s.claudeHome)      ([]claude.SessionFacet, error)
claude.ParseAgentTasks(s.claudeHome)     ([]claude.AgentTask, error)
analyzer.AnalyzeCommits(sessions []claude.SessionMeta) analyzer.CommitAnalysis
```

For `HasClaudeMD`: use `os.Stat(filepath.Join(projectPath, "CLAUDE.md"))` where
`projectPath` is taken from the matching session's `ProjectPath` field.

If the `project` argument is omitted, use the most recent session's project
(`filepath.Base(sessions[0].ProjectPath)` after sorting descending by `StartTime`).

#### Agent B — Result types and handler signature (`suggest_tools.go`)

```go
package mcp

// SuggestionItem holds a single ranked suggestion for the get_suggestions tool.
type SuggestionItem struct {
    Category    string  `json:"category"`
    Priority    int     `json:"priority"`
    Title       string  `json:"title"`
    Description string  `json:"description"`
    ImpactScore float64 `json:"impact_score"`
}

// SuggestionsResult holds the full get_suggestions response.
type SuggestionsResult struct {
    Suggestions []SuggestionItem `json:"suggestions"`
    TotalCount  int              `json:"total_count"`
    Project     string           `json:"project,omitempty"`
}

func (s *Server) handleGetSuggestions(args json.RawMessage) (any, error)
```

Agent B builds its own `suggest.AnalysisContext` inline (do NOT call
`app.buildAnalysisContext` — that's in a sibling package). Build it with:

```go
suggest.NewEngine()                                    // from internal/suggest
suggest.AnalysisContext{...}                           // from internal/suggest/types.go
claude.ParseAllSessionMeta(s.claudeHome)
claude.ParseAllFacets(s.claudeHome)
claude.ParseSettings(s.claudeHome)
claude.ListCommands(s.claudeHome)
claude.ParsePlugins(s.claudeHome)
claude.ParseAgentTasks(s.claudeHome)
claude.ParseStatsCache(s.claudeHome)
analyzer.AnalyzeCommits(sessions)
```

**No `scanner.DiscoverProjects` in MCP mode.** Infer project list from session
metadata: group sessions by `claude.NormalizePath(session.ProjectPath)`, compute
`filepath.Base(projectPath)` as the project name, set `HasClaudeMD` via `os.Stat`.
Use `0.3` as the hardcoded recurring friction threshold (no config available in MCP).

#### Agent C — Result types and handler signatures (`analytics_tools.go`)

```go
package mcp

// AgentPerformanceResult holds the get_agent_performance response.
// Maps directly from analyzer.AgentPerformance.
type AgentPerformanceResult struct {
    TotalAgents       int                            `json:"total_agents"`
    SuccessRate       float64                        `json:"success_rate"`
    KillRate          float64                        `json:"kill_rate"`
    BackgroundRatio   float64                        `json:"background_ratio"`
    AvgDurationMs     float64                        `json:"avg_duration_ms"`
    AvgTokensPerAgent float64                        `json:"avg_tokens_per_agent"`
    ParallelSessions  int                            `json:"parallel_sessions"`
    ByType            map[string]AgentTypePerfDetail `json:"by_type"`
}

// AgentTypePerfDetail holds per-type performance stats.
type AgentTypePerfDetail struct {
    Count         int     `json:"count"`
    SuccessRate   float64 `json:"success_rate"`
    AvgDurationMs float64 `json:"avg_duration_ms"`
    AvgTokens     float64 `json:"avg_tokens"`
}

// EffectivenessEntry holds before/after CLAUDE.md effectiveness for one project.
type EffectivenessEntry struct {
    Project          string  `json:"project"`
    Verdict          string  `json:"verdict"`
    Score            int     `json:"score"`
    FrictionDelta    float64 `json:"friction_delta"`
    ToolErrorDelta   float64 `json:"tool_error_delta"`
    BeforeSessions   int     `json:"before_sessions"`
    AfterSessions    int     `json:"after_sessions"`
    ChangeDetectedAt string  `json:"change_detected_at"`
}

// AllEffectivenessResult holds the get_effectiveness response.
type AllEffectivenessResult struct {
    Projects []EffectivenessEntry `json:"projects"`
}

func (s *Server) handleGetAgentPerformance(args json.RawMessage) (any, error)
func (s *Server) handleGetEffectiveness(args json.RawMessage) (any, error)
```

Existing functions Agent C calls:
```go
claude.ParseAgentTasks(s.claudeHome)                       ([]claude.AgentTask, error)
claude.ParseAllSessionMeta(s.claudeHome)                   ([]claude.SessionMeta, error)
analyzer.AnalyzeAgents(tasks []claude.AgentTask)           analyzer.AgentPerformance
analyzer.AnalyzeEffectiveness(                             analyzer.EffectivenessResult
    projectPath string,
    claudeMDModTime time.Time,
    sessions []claude.SessionMeta,
    facets []claude.SessionFacet,
    pricing analyzer.ModelPricing,
    ratio analyzer.CacheRatio,
)
```

For `handleGetEffectiveness`: iterate over session project paths, detect CLAUDE.md
mod time via `os.Stat`, call `analyzer.AnalyzeEffectiveness` per project. See
`internal/app/metrics.go` `runMetrics` for the pattern (search for
`AnalyzeEffectivenessAllProjects` or similar — read that file for the exact call
pattern before implementing).

#### Agent D — Result types and handler signature (`friction_tools.go`)

```go
package mcp

// SessionFrictionResult holds the get_session_friction response.
type SessionFrictionResult struct {
    SessionID       string         `json:"session_id"`
    FrictionCounts  map[string]int `json:"friction_counts"`
    TotalFriction   int            `json:"total_friction"`
    TopFrictionType string         `json:"top_friction_type,omitempty"`
}

func (s *Server) handleGetSessionFriction(args json.RawMessage) (any, error)
```

Existing functions Agent D calls:
```go
claude.ParseAllFacets(s.claudeHome) ([]claude.SessionFacet, error)
// claude.SessionFacet has:
//   SessionID     string
//   FrictionCounts map[string]int
```

Filter facets by `session_id`. Sum `FrictionCounts`, find the key with the highest
count for `TopFrictionType`. If no facet exists for the given session, return an empty
result (not an error) with `total_friction: 0` and an empty `friction_counts` map.

---

### File Ownership

| File | Agent | Wave | Action |
|------|-------|------|--------|
| `internal/mcp/health_tools.go` | A | 1 | create |
| `internal/mcp/health_tools_test.go` | A | 1 | create |
| `internal/mcp/suggest_tools.go` | B | 1 | create |
| `internal/mcp/suggest_tools_test.go` | B | 1 | create |
| `internal/mcp/analytics_tools.go` | C | 1 | create |
| `internal/mcp/analytics_tools_test.go` | C | 1 | create |
| `internal/mcp/friction_tools.go` | D | 1 | create |
| `internal/mcp/friction_tools_test.go` | D | 1 | create |
| `internal/mcp/tools.go` | Orchestrator | post-merge | add 5 registerTool calls to addTools |

No existing files are modified by any agent.

---

### Wave Structure

```
Wave 1: [A] [B] [C] [D]    <- 4 parallel agents, all independent
         |
         | (all complete → orchestrator merges → adds registerTool calls to tools.go)
         |
         post-merge: gofmt -w . && golangci-lint run --fix && go test ./...
```

---

### Agent Prompts

---

#### Agent A: `get_project_health`

```
# Wave 1 Agent A: get_project_health MCP tool

You are Wave 1 Agent A. Create internal/mcp/health_tools.go implementing the
get_project_health MCP tool handler and its result types.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a 2>/dev/null || true
```

**Step 2: Verify isolation**

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
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || { echo "ISOLATION FAILURE: Worktree not in git records"; exit 1; }
echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

If verification fails: write the failure to your completion report and stop.

## 1. File Ownership

You own these files only:
- `internal/mcp/health_tools.go` — create
- `internal/mcp/health_tools_test.go` — create

Do not touch any other files.

## 2. Interfaces You Must Implement

```go
// In health_tools.go:

type ProjectHealthResult struct {
    Project          string                      `json:"project"`
    SessionCount     int                         `json:"session_count"`
    FrictionRate     float64                     `json:"friction_rate"`
    TopFriction      []string                    `json:"top_friction_types"`
    AvgToolErrors    float64                     `json:"avg_tool_errors_per_session"`
    ZeroCommitRate   float64                     `json:"zero_commit_rate"`
    AgentSuccessRate float64                     `json:"agent_success_rate"`
    HasClaudeMD      bool                        `json:"has_claude_md"`
    ByAgentType      map[string]AgentTypeSummary `json:"agent_performance_by_type"`
}

type AgentTypeSummary struct {
    Count       int     `json:"count"`
    SuccessRate float64 `json:"success_rate"`
}

func (s *Server) handleGetProjectHealth(args json.RawMessage) (any, error)
```

The registration call is handled by the orchestrator post-merge — do not modify tools.go.

## 3. Interfaces You May Call

```go
// internal/claude package:
claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
claude.ParseAllFacets(claudeHome string) ([]claude.SessionFacet, error)
claude.ParseAgentTasks(claudeHome string) ([]claude.AgentTask, error)
claude.NormalizePath(path string) string

// internal/analyzer package:
analyzer.AnalyzeCommits(sessions []claude.SessionMeta) analyzer.CommitAnalysis
// analyzer.CommitAnalysis has: ZeroCommitRate float64, AvgCommitsPerSession float64

// Standard library:
filepath.Base(path string) string
os.Stat(path string) (os.FileInfo, error)
```

Read `internal/claude/types.go` for the `SessionMeta`, `SessionFacet`, and `AgentTask`
struct definitions before implementing.

## 4. What to Implement

`handleGetProjectHealth` accepts an optional `project` string argument. If omitted,
use the most recent session's project (sort sessions descending by `StartTime`, take
`filepath.Base(sessions[0].ProjectPath)`).

**Project filtering:** Match sessions where `filepath.Base(session.ProjectPath) == project`.
Use `claude.NormalizePath` for path comparison when the full path is available.

**FrictionRate:** fraction of project sessions that have any friction events in facets.
Build a set of `SessionID`s for the project's sessions, then count how many appear in
facets with `len(facet.FrictionCounts) > 0`.

**TopFriction:** collect all friction type keys from all project facets, count by type,
sort descending, take the top 3 (or fewer if less exist).

**AvgToolErrors:** average of `session.ToolErrors` across project sessions.

**ZeroCommitRate:** call `analyzer.AnalyzeCommits` on the project's sessions subset
and use `result.ZeroCommitRate`.

**AgentSuccessRate:** filter `AgentTask`s by project (match `task.SessionID` against
project session IDs), compute fraction with `task.Status == "completed"`.

**ByAgentType:** same filtered agent tasks, group by `task.AgentType`, compute count
and success rate per type.

**HasClaudeMD:** take the `ProjectPath` from any project session, check
`os.Stat(filepath.Join(projectPath, "CLAUDE.md"))` — `true` if no error.

Return an empty `ProjectHealthResult` (not an error) if no sessions exist for the
requested project, with `SessionCount: 0`.

## 5. Tests to Write

1. `TestGetProjectHealth_EmptyDir` — no session data returns empty result without error
2. `TestGetProjectHealth_DefaultsToMostRecent` — no project arg uses most recent session's project
3. `TestGetProjectHealth_FiltersByProject` — sessions from two projects, only target project's data returned
4. `TestGetProjectHealth_FrictionRate` — 2 sessions, 1 with friction: rate = 0.5
5. `TestGetProjectHealth_AgentSuccessRate` — 3 agents: 2 completed, 1 failed → 0.667
6. `TestGetProjectHealth_TopFriction` — returns top 3 friction types sorted by frequency
7. `TestGetProjectHealth_UnknownProject` — unknown project name returns zero-value result

Use the existing test helpers from `tools_test.go`: `writeSessionMeta`, `writeFacet`,
`newTestServer`, `callTool`. Add a helper `writeTranscriptForHealth` if you need agent
span data (or use `writeTranscriptJSONL` from `saw_tools_test.go`).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetProjectHealth -v
```

All tests must pass. Do not run linter auto-fix.

## 7. Constraints

- Do not import `internal/app` — it would create an import cycle (app imports mcp indirectly via config).
- Do not modify `tools.go` — registration is orchestrator-owned.
- Return `(any, error)` from the handler, matching the existing handler signature.
- All errors from data loading are non-fatal: if `ParseAgentTasks` fails, continue
  with empty agent data rather than returning an error.
- TopFriction slice should be `[]string{}` (not nil) when empty, for clean JSON output.

## 8. Report

Commit before reporting:
```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
git add .
git commit -m "wave1-agent-a: add get_project_health MCP tool"
```

Append to `docs/IMPL-mcp-self-model-tools.md` under `### Agent A — Completion Report`.
```

---

#### Agent B: `get_suggestions`

```
# Wave 1 Agent B: get_suggestions MCP tool

You are Wave 1 Agent B. Create internal/mcp/suggest_tools.go implementing the
get_suggestions MCP tool handler and its result types.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b 2>/dev/null || true

ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then echo "ISOLATION FAILURE"; exit 1; fi

ACTUAL_BRANCH=$(git branch --show-current)
if [ "$ACTUAL_BRANCH" != "wave1-agent-b" ]; then echo "ISOLATION FAILURE"; exit 1; fi

git worktree list | grep -q "wave1-agent-b" || { echo "ISOLATION FAILURE"; exit 1; }
echo "✓ Isolation verified"
```

## 1. File Ownership

- `internal/mcp/suggest_tools.go` — create
- `internal/mcp/suggest_tools_test.go` — create

Do not touch any other files.

## 2. Interfaces You Must Implement

```go
type SuggestionItem struct {
    Category    string  `json:"category"`
    Priority    int     `json:"priority"`
    Title       string  `json:"title"`
    Description string  `json:"description"`
    ImpactScore float64 `json:"impact_score"`
}

type SuggestionsResult struct {
    Suggestions []SuggestionItem `json:"suggestions"`
    TotalCount  int              `json:"total_count"`
    Project     string           `json:"project,omitempty"`
}

func (s *Server) handleGetSuggestions(args json.RawMessage) (any, error)
```

## 3. Interfaces You May Call

```go
// internal/suggest package — read internal/suggest/types.go and engine.go first:
suggest.NewEngine() *suggest.Engine
(*suggest.Engine).Run(ctx *suggest.AnalysisContext) []suggest.Suggestion
// suggest.Suggestion has: Category, Priority string/int, Title, Description string, ImpactScore float64

// internal/claude package:
claude.ParseAllSessionMeta(s.claudeHome) ([]claude.SessionMeta, error)
claude.ParseAllFacets(s.claudeHome)      ([]claude.SessionFacet, error)
claude.ParseSettings(s.claudeHome)       (*claude.GlobalSettings, error)
claude.ListCommands(s.claudeHome)        ([]claude.CommandFile, error)
claude.ParsePlugins(s.claudeHome)        ([]claude.PluginEntry, error)
claude.ParseAgentTasks(s.claudeHome)     ([]claude.AgentTask, error)
claude.ParseStatsCache(s.claudeHome)     (*claude.StatsCache, error)
claude.NormalizePath(path string) string

// internal/analyzer package:
analyzer.AnalyzeCommits(sessions []claude.SessionMeta) analyzer.CommitAnalysis
analyzer.EstimateCosts(sc claude.StatsCache, model string, sessions int, commits int) analyzer.CostEstimate
// analyzer.CostEstimate has: CacheSavingsPercent float64, TotalCost float64
```

Read `internal/suggest/types.go` in full before implementing — particularly
`AnalysisContext` and `ProjectContext` field names.

## 4. What to Implement

`handleGetSuggestions` accepts optional `project` (string) and `limit` (int, default 5,
max 20) arguments.

**Build AnalysisContext without scanner.DiscoverProjects.** Infer projects from session
metadata: group sessions by `claude.NormalizePath(session.ProjectPath)`, then for each
unique project path construct a `suggest.ProjectContext` from session-derived data.

```go
// Pattern for building project contexts from sessions:
projectSessions := make(map[string][]claude.SessionMeta)
for _, s := range sessions {
    key := claude.NormalizePath(s.ProjectPath)
    projectSessions[key] = append(projectSessions[key], s)
}
// For each key: name = filepath.Base(key), HasClaudeMD = os.Stat check,
// SessionCount = len(projectSessions[key]), etc.
```

Use `0.3` as the hardcoded recurring friction threshold (RecurringThreshold).

For `AgentTypeStats` in `AnalysisContext`: compute from `ParseAgentTasks` the same
way `app/suggest.go`'s `buildAnalysisContext` does (read it as a reference).

**Filter by project:** if `project` arg is set, run suggestions through a filter keeping
only those whose `Title` or `Description` contains the project name (same logic as
`app/suggest.go`'s `filterByProject`).

**Apply limit** before returning.

## 5. Tests to Write

1. `TestGetSuggestions_EmptyData` — no sessions/facets, returns empty suggestions without error
2. `TestGetSuggestions_DefaultLimit` — more than 5 suggestions available, default returns 5
3. `TestGetSuggestions_CustomLimit` — limit=2 returns at most 2
4. `TestGetSuggestions_ProjectFilter` — project filter reduces results to project-specific items
5. `TestGetSuggestions_SortedByImpact` — returned suggestions are in descending impact order
6. `TestGetSuggestions_MissingClaudeMD` — session with no CLAUDE.md generates a suggestion

For tests, you can write minimal session-meta files with `writeSessionMeta` from
`tools_test.go`. The suggest engine is deterministic given the same input.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetSuggestions -v
```

## 7. Constraints

- Do not import `internal/app`. Build the AnalysisContext inline from `suggest`, `claude`,
  and `analyzer` packages only.
- Do not modify `tools.go`.
- All data loading errors are non-fatal — continue with zero values.
- `SuggestionsResult.Suggestions` must be `[]SuggestionItem{}` (not nil) when empty.
- `TotalCount` is the count before limit is applied (total available suggestions).

## 8. Report

```bash
git add .
git commit -m "wave1-agent-b: add get_suggestions MCP tool"
```

Append to `docs/IMPL-mcp-self-model-tools.md` under `### Agent B — Completion Report`.
```

---

#### Agent C: `get_agent_performance` + `get_effectiveness`

```
# Wave 1 Agent C: get_agent_performance and get_effectiveness MCP tools

You are Wave 1 Agent C. Create internal/mcp/analytics_tools.go implementing two
MCP tool handlers: get_agent_performance and get_effectiveness.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-c 2>/dev/null || true

ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-c"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then echo "ISOLATION FAILURE"; exit 1; fi
ACTUAL_BRANCH=$(git branch --show-current)
if [ "$ACTUAL_BRANCH" != "wave1-agent-c" ]; then echo "ISOLATION FAILURE"; exit 1; fi
git worktree list | grep -q "wave1-agent-c" || { echo "ISOLATION FAILURE"; exit 1; }
echo "✓ Isolation verified"
```

## 1. File Ownership

- `internal/mcp/analytics_tools.go` — create
- `internal/mcp/analytics_tools_test.go` — create

Do not touch any other files.

## 2. Interfaces You Must Implement

```go
type AgentPerformanceResult struct {
    TotalAgents       int                            `json:"total_agents"`
    SuccessRate       float64                        `json:"success_rate"`
    KillRate          float64                        `json:"kill_rate"`
    BackgroundRatio   float64                        `json:"background_ratio"`
    AvgDurationMs     float64                        `json:"avg_duration_ms"`
    AvgTokensPerAgent float64                        `json:"avg_tokens_per_agent"`
    ParallelSessions  int                            `json:"parallel_sessions"`
    ByType            map[string]AgentTypePerfDetail `json:"by_type"`
}

type AgentTypePerfDetail struct {
    Count         int     `json:"count"`
    SuccessRate   float64 `json:"success_rate"`
    AvgDurationMs float64 `json:"avg_duration_ms"`
    AvgTokens     float64 `json:"avg_tokens"`
}

type EffectivenessEntry struct {
    Project          string  `json:"project"`
    Verdict          string  `json:"verdict"`
    Score            int     `json:"score"`
    FrictionDelta    float64 `json:"friction_delta"`
    ToolErrorDelta   float64 `json:"tool_error_delta"`
    BeforeSessions   int     `json:"before_sessions"`
    AfterSessions    int     `json:"after_sessions"`
    ChangeDetectedAt string  `json:"change_detected_at"` // RFC3339
}

type AllEffectivenessResult struct {
    Projects []EffectivenessEntry `json:"projects"`
}

func (s *Server) handleGetAgentPerformance(args json.RawMessage) (any, error)
func (s *Server) handleGetEffectiveness(args json.RawMessage) (any, error)
```

## 3. Interfaces You May Call

```go
// internal/claude:
claude.ParseAgentTasks(s.claudeHome) ([]claude.AgentTask, error)
claude.ParseAllSessionMeta(s.claudeHome) ([]claude.SessionMeta, error)
claude.ParseAllFacets(s.claudeHome) ([]claude.SessionFacet, error)
claude.ParseStatsCache(s.claudeHome) (*claude.StatsCache, error)

// internal/analyzer:
analyzer.AnalyzeAgents(tasks []claude.AgentTask) analyzer.AgentPerformance
// analyzer.AgentPerformance maps directly to AgentPerformanceResult fields.
// analyzer.AgentPerformance.ByType is map[string]analyzer.AgentTypeStats

analyzer.AnalyzeEffectiveness(
    projectPath string,
    claudeMDModTime time.Time,
    sessions []claude.SessionMeta,
    facets []claude.SessionFacet,
    pricing analyzer.ModelPricing,
    ratio analyzer.CacheRatio,
) analyzer.EffectivenessResult
// Read internal/analyzer/effectiveness.go for full signature before using.

analyzer.DefaultPricing["sonnet"]   // analyzer.ModelPricing
s.loadCacheRatio()                  // already defined on *Server in tools.go
```

Read `internal/analyzer/effectiveness.go` in full before implementing
`handleGetEffectiveness`. Also read `internal/app/metrics.go` to see how
the CLI iterates over projects to call `AnalyzeEffectiveness` — use the same
pattern.

## 4. What to Implement

**`handleGetAgentPerformance`:** No arguments. Call `claude.ParseAgentTasks`,
call `analyzer.AnalyzeAgents`, map the result to `AgentPerformanceResult`.
Map `analyzer.AgentTypeStats` fields directly to `AgentTypePerfDetail`.

**`handleGetEffectiveness`:** No arguments. Load all sessions, facets, and
the stats cache. Discover unique project paths from sessions (same
`claude.NormalizePath` grouping as other handlers). For each project path:
  1. Check `os.Stat(filepath.Join(projectPath, "CLAUDE.md"))` for existence
     and mod time. Skip projects without CLAUDE.md (no change to measure).
  2. Call `analyzer.AnalyzeEffectiveness(projectPath, modTime, projectSessions,
     projectFacets, pricing, ratio)`.
  3. Map the result to `EffectivenessEntry`.

Set `ChangeDetectedAt` to `result.ChangeDetectedAt.Format(time.RFC3339)`.
Skip projects where `result.BeforeSessions == 0 && result.AfterSessions == 0`.
Return empty `Projects: []EffectivenessEntry{}` (not nil) if none found.

## 5. Tests to Write

For `get_agent_performance`:
1. `TestGetAgentPerformance_NoAgents` — no transcript data returns zero-value result
2. `TestGetAgentPerformance_SuccessRate` — 3 agents: 2 completed, 1 failed → SuccessRate 0.667
3. `TestGetAgentPerformance_ByType` — agents of different types produce correct ByType map

For `get_effectiveness`:
4. `TestGetEffectiveness_NoProjects` — no session data returns empty Projects list
5. `TestGetEffectiveness_NoCLAUDEMD` — project without CLAUDE.md is excluded from results
6. `TestGetEffectiveness_ReturnsVerdict` — project with sessions before/after CLAUDE.md change

Use `writeTranscriptJSONL` from `saw_tools_test.go` to create agent span data.
Use `writeSessionMeta` and `writeFacet` from `tools_test.go` for session and facet data.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-c
go build ./...
go vet ./...
go test ./internal/mcp -run "TestGetAgent|TestGetEffect" -v
```

## 7. Constraints

- `handleGetAgentPerformance` accepts `noArgsSchema` (ignore any args passed).
- `handleGetEffectiveness` accepts `noArgsSchema` (ignore any args passed).
- Do not modify `tools.go`.
- All data loading is non-fatal: return zero-value results on parse errors.
- `ByType` map must be `map[string]AgentTypePerfDetail{}` (not nil) when empty.

## 8. Report

```bash
git add .
git commit -m "wave1-agent-c: add get_agent_performance and get_effectiveness MCP tools"
```

Append to `docs/IMPL-mcp-self-model-tools.md` under `### Agent C — Completion Report`.
```

---

#### Agent D: `get_session_friction`

```
# Wave 1 Agent D: get_session_friction MCP tool

You are Wave 1 Agent D. Create internal/mcp/friction_tools.go implementing the
get_session_friction MCP tool handler and its result types.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-d 2>/dev/null || true

ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-d"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then echo "ISOLATION FAILURE"; exit 1; fi
ACTUAL_BRANCH=$(git branch --show-current)
if [ "$ACTUAL_BRANCH" != "wave1-agent-d" ]; then echo "ISOLATION FAILURE"; exit 1; fi
git worktree list | grep -q "wave1-agent-d" || { echo "ISOLATION FAILURE"; exit 1; }
echo "✓ Isolation verified"
```

## 1. File Ownership

- `internal/mcp/friction_tools.go` — create
- `internal/mcp/friction_tools_test.go` — create

Do not touch any other files.

## 2. Interfaces You Must Implement

```go
type SessionFrictionResult struct {
    SessionID       string         `json:"session_id"`
    FrictionCounts  map[string]int `json:"friction_counts"`
    TotalFriction   int            `json:"total_friction"`
    TopFrictionType string         `json:"top_friction_type,omitempty"`
}

func (s *Server) handleGetSessionFriction(args json.RawMessage) (any, error)
```

## 3. Interfaces You May Call

```go
// internal/claude:
claude.ParseAllFacets(s.claudeHome) ([]claude.SessionFacet, error)
// claude.SessionFacet:
//   SessionID     string
//   FrictionCounts map[string]int
```

Read `internal/claude/facets.go` to understand the exact struct fields.

## 4. What to Implement

`handleGetSessionFriction` requires a `session_id` string argument. Return an
error if it is empty or missing.

Call `claude.ParseAllFacets`. Find the facet whose `SessionID` matches the
requested session ID (exact string match). If no facet is found, return a
`SessionFrictionResult` with `SessionID` set, `FrictionCounts: map[string]int{}`,
`TotalFriction: 0`, and no error — an absent facet means no friction was recorded
yet, which is valid.

If found: sum all values in `FrictionCounts` for `TotalFriction`. Find the key with
the highest value for `TopFrictionType` (break ties alphabetically for determinism).
If `FrictionCounts` is empty or nil, set `TopFrictionType` to `""`.

## 5. Tests to Write

1. `TestGetSessionFriction_RequiresSessionID` — empty session_id returns error
2. `TestGetSessionFriction_NotFound` — unknown session_id returns empty result, no error
3. `TestGetSessionFriction_NoFriction` — session with empty FrictionCounts returns TotalFriction=0
4. `TestGetSessionFriction_WithFriction` — session with friction: correct TotalFriction and TopFrictionType
5. `TestGetSessionFriction_TopFrictionType` — multiple friction types, top is the highest-count one
6. `TestGetSessionFriction_TieBreak` — equal counts: TopFrictionType is lexicographically first

Use `writeFacet` from `tools_test.go` to set up test data.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-d
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetSessionFriction -v
```

## 7. Constraints

- `session_id` is required. Return `errors.New("session_id is required")` if absent.
- If no facet exists for the session, return empty result (not error).
- `FrictionCounts` must be `map[string]int{}` (not nil) in the result.
- Do not modify `tools.go`.

## 8. Report

```bash
git add .
git commit -m "wave1-agent-d: add get_session_friction MCP tool"
```

Append to `docs/IMPL-mcp-self-model-tools.md` under `### Agent D — Completion Report`.
```

---

### Wave Execution Loop

**After Wave 1 completes:**

1. Read each agent's completion report from their sections below.
2. Merge all four worktrees:
   ```bash
   cd /Users/dayna.blackwell/code/claudewatch
   git merge wave1-agent-a wave1-agent-b wave1-agent-c wave1-agent-d
   ```
3. Add the 5 `registerTool` calls to `addTools` in `internal/mcp/tools.go`
   (see Interface Contracts → Orchestrator post-merge additions section above).
4. Run linter auto-fix:
   ```bash
   gofmt -w .
   golangci-lint run --fix
   git diff --name-only
   # Commit style changes if any
   ```
5. Run full verification:
   ```bash
   go build ./...
   go vet ./...
   go test ./...
   ```
6. Fix any integration issues, then commit the wave result.

---

### Status

- [ ] Wave 1 Agent A — `get_project_health` (health_tools.go)
- [ ] Wave 1 Agent B — `get_suggestions` (suggest_tools.go)
- [ ] Wave 1 Agent C — `get_agent_performance` + `get_effectiveness` (analytics_tools.go)
- [ ] Wave 1 Agent D — `get_session_friction` (friction_tools.go)
- [ ] Orchestrator — merge, add registerTool calls, lint auto-fix, full test run

---

### Agent A — Completion Report

**Status:** Complete

**Files created:**
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a/internal/mcp/health_tools.go`
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a/internal/mcp/health_tools_test.go`

**Test count:** 7 tests

**Tests written:**
1. `TestGetProjectHealth_EmptyDir` — no session data returns empty result without error
2. `TestGetProjectHealth_DefaultsToMostRecent` — no project arg uses most recent session's project
3. `TestGetProjectHealth_FiltersByProject` — sessions from two projects, only target project's data returned
4. `TestGetProjectHealth_FrictionRate` — 2 sessions, 1 with friction: rate = 0.5
5. `TestGetProjectHealth_AgentSuccessRate` — 3 agents: 2 completed, 1 failed => 0.667
6. `TestGetProjectHealth_TopFriction` — returns top 3 friction types sorted by frequency
7. `TestGetProjectHealth_UnknownProject` — unknown project name returns zero-value result

**Verification gate status:** PASS

```
=== RUN   TestGetProjectHealth_EmptyDir
--- PASS: TestGetProjectHealth_EmptyDir (0.00s)
=== RUN   TestGetProjectHealth_DefaultsToMostRecent
--- PASS: TestGetProjectHealth_DefaultsToMostRecent (0.07s)
=== RUN   TestGetProjectHealth_FiltersByProject
--- PASS: TestGetProjectHealth_FiltersByProject (0.06s)
=== RUN   TestGetProjectHealth_FrictionRate
--- PASS: TestGetProjectHealth_FrictionRate (0.05s)
=== RUN   TestGetProjectHealth_AgentSuccessRate
--- PASS: TestGetProjectHealth_AgentSuccessRate (0.05s)
=== RUN   TestGetProjectHealth_TopFriction
--- PASS: TestGetProjectHealth_TopFriction (0.07s)
=== RUN   TestGetProjectHealth_UnknownProject
--- PASS: TestGetProjectHealth_UnknownProject (0.00s)
PASS
ok  	github.com/blackwell-systems/claudewatch/internal/mcp	0.839s
```

`go build ./...` and `go vet ./...` both pass with no output.

**Deviations from spec:**
- Tool registration is orchestrator-owned. Tests use a local `addHealthTool` helper that registers the handler on the test server without touching `tools.go`.
- Agent task status mapping uses transcript `is_error` fields via `ParseAgentTasks`: `is_error:false` => "completed", `is_error:true` => "failed". Tests use a `writeAgentTaskTranscript` helper to create transcript files with the correct format.

**Commit:** `wave1-agent-a: add get_project_health MCP tool`

---

### Agent D — Completion Report

**Status:** Complete

**Files created:**
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-d/internal/mcp/friction_tools.go`
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-d/internal/mcp/friction_tools_test.go`

**Test count:** 6 tests

**Tests written:**
1. `TestGetSessionFriction_RequiresSessionID` — empty session_id returns error
2. `TestGetSessionFriction_NotFound` — unknown session_id returns empty result, no error
3. `TestGetSessionFriction_NoFriction` — session with empty FrictionCounts returns TotalFriction=0
4. `TestGetSessionFriction_WithFriction` — session with friction: correct TotalFriction and TopFrictionType
5. `TestGetSessionFriction_TopFrictionType` — multiple friction types, top is the highest-count one
6. `TestGetSessionFriction_TieBreak` — equal counts: TopFrictionType is lexicographically first

**Verification gate status:** PASS

```
=== RUN   TestGetSessionFriction_RequiresSessionID
--- PASS: TestGetSessionFriction_RequiresSessionID (0.00s)
=== RUN   TestGetSessionFriction_NotFound
--- PASS: TestGetSessionFriction_NotFound (0.00s)
=== RUN   TestGetSessionFriction_NoFriction
--- PASS: TestGetSessionFriction_NoFriction (0.00s)
=== RUN   TestGetSessionFriction_WithFriction
--- PASS: TestGetSessionFriction_WithFriction (0.00s)
=== RUN   TestGetSessionFriction_TopFrictionType
--- PASS: TestGetSessionFriction_TopFrictionType (0.00s)
=== RUN   TestGetSessionFriction_TieBreak
--- PASS: TestGetSessionFriction_TieBreak (0.01s)
PASS
ok  	github.com/blackwell-systems/claudewatch/internal/mcp	0.487s
```

`go build ./...` and `go vet ./...` both pass with no output.

**Deviations from spec:** None. The tool registration in `addTools` was intentionally left to the orchestrator post-merge (per constraint: "Do not modify `tools.go`"). Tests call `s.handleGetSessionFriction` directly and do not require tool registration to pass.

**Commit:** `wave1-agent-d: add get_session_friction MCP tool`

---

### Agent B — Completion Report

**Status:** Complete

**Files created:**
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b/internal/mcp/suggest_tools.go`
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b/internal/mcp/suggest_tools_test.go`

**Test count:** 6 tests

**Tests written:**
1. `TestGetSuggestions_EmptyData` — no sessions/facets, verifies non-nil Suggestions slice and consistent TotalCount without error
2. `TestGetSuggestions_DefaultLimit` — 10 projects without CLAUDE.md produce >5 suggestions; default limit returns exactly 5
3. `TestGetSuggestions_CustomLimit` — limit=2 returns at most 2 items
4. `TestGetSuggestions_ProjectFilter` — project="alpha" filter: every returned suggestion mentions "alpha" in Title or Description
5. `TestGetSuggestions_SortedByImpact` — returned suggestions are in non-increasing ImpactScore order
6. `TestGetSuggestions_MissingClaudeMD` — session pointing to a dir without CLAUDE.md yields a CLAUDE.md suggestion

**Verification gate status:** PASS

```
=== RUN   TestGetSuggestions_EmptyData
--- PASS: TestGetSuggestions_EmptyData (0.00s)
=== RUN   TestGetSuggestions_DefaultLimit
--- PASS: TestGetSuggestions_DefaultLimit (0.33s)
=== RUN   TestGetSuggestions_CustomLimit
--- PASS: TestGetSuggestions_CustomLimit (0.17s)
=== RUN   TestGetSuggestions_ProjectFilter
--- PASS: TestGetSuggestions_ProjectFilter (0.03s)
=== RUN   TestGetSuggestions_SortedByImpact
--- PASS: TestGetSuggestions_SortedByImpact (0.12s)
=== RUN   TestGetSuggestions_MissingClaudeMD
--- PASS: TestGetSuggestions_MissingClaudeMD (0.00s)
PASS
ok  	github.com/blackwell-systems/claudewatch/internal/mcp	1.194s
```

`go build ./...` and `go vet ./...` both pass with no output.

**Deviations from spec:**
- `TestGetSuggestions_EmptyData`: the spec says "returns empty suggestions" but the `HookGaps` rule fires whenever `HookCount == 0` (even with zero sessions), producing 1 suggestion. The test was updated to verify structural invariants (non-nil slice, `TotalCount >= len(Suggestions)`) rather than asserting an exact count of zero, which would be incorrect.
- Tool registration is orchestrator-owned. Tests call `s.handleGetSuggestions` directly rather than routing through `callTool`, consistent with how Agent A and D structured their tests.
- `ClaudeMDSectionCorrelation` is left nil in the built `AnalysisContext` because it requires `analyzer.AnalyzeClaudeMDEffectiveness` which also requires `scanner.DiscoverProjects` output (the `projects` slice with `HasClaudeMD` and `MissingSections` populated). This is acceptable — the corresponding suggest rules will simply not fire when the correlation map is nil/empty.

**Commit:** `wave1-agent-b: add get_suggestions MCP tool`

---

### Agent C — Completion Report

**Status:** Complete

**Files created:**
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-c/internal/mcp/analytics_tools.go`
- `/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-c/internal/mcp/analytics_tools_test.go`

**Test count:** 6 tests

**Tests written:**
1. `TestGetAgentPerformance_NoAgents` — no transcript data returns zero-value result with non-nil ByType map
2. `TestGetAgentPerformance_SuccessRate` — 3 agents (2 completed, 1 failed) → SuccessRate ≈ 0.667
3. `TestGetAgentPerformance_ByType` — agents of type "coder" and "builder" produce correct ByType map with per-type counts and success rates
4. `TestGetEffectiveness_NoProjects` — no session data returns non-nil empty Projects slice
5. `TestGetEffectiveness_NoCLAUDEMD` — project without CLAUDE.md is excluded from results
6. `TestGetEffectiveness_ReturnsVerdict` — project with 3 sessions before and 3 sessions after CLAUDE.md change is included with a non-empty verdict

**Verification gate status:** PASS

```
=== RUN   TestGetAgentPerformance_NoAgents
--- PASS: TestGetAgentPerformance_NoAgents (0.00s)
=== RUN   TestGetAgentPerformance_SuccessRate
--- PASS: TestGetAgentPerformance_SuccessRate (0.01s)
=== RUN   TestGetAgentPerformance_ByType
--- PASS: TestGetAgentPerformance_ByType (0.00s)
=== RUN   TestGetEffectiveness_NoProjects
--- PASS: TestGetEffectiveness_NoProjects (0.00s)
=== RUN   TestGetEffectiveness_NoCLAUDEMD
--- PASS: TestGetEffectiveness_NoCLAUDEMD (0.01s)
=== RUN   TestGetEffectiveness_ReturnsVerdict
--- PASS: TestGetEffectiveness_ReturnsVerdict (0.01s)
PASS
ok  	github.com/blackwell-systems/claudewatch/internal/mcp	0.746s
```

`go build ./...` and `go vet ./...` both pass with no output.

**Deviations from spec:**
- Tool registration: the spec constraint "Do not touch any other files" and "Do not modify tools.go" conflict with the requirement that `get_agent_performance` and `get_effectiveness` be callable via `callTool`. Resolution: `addAnalyticsTools(s *Server)` is defined in `analytics_tools.go` and called from a test-local helper `newAnalyticsTestServer` in the test file. For production wiring, `addAnalyticsTools` is ready to be called from `addTools` in `tools.go` by the merge orchestrator.
- `handleGetEffectiveness` passes all `facets` (not just project-filtered facets) to `analyzer.AnalyzeEffectiveness`. This matches how `AnalyzeEffectiveness` internally indexes facets by session ID, making project-level pre-filtering unnecessary.
- `ByType` map is initialized via `make()` before the range over `perf.ByType`, ensuring it is always non-nil even when the analyzer returns an empty map.

**Commit:** `wave1-agent-c: add get_agent_performance and get_effectiveness MCP tools`
