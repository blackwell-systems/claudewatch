# IMPL: Factor Analysis

**Feature slug:** `factor-analysis`
**ROADMAP ref:** Tier 2 — "Factor analysis" (correlate session attributes against outcomes)
**Deliverables:**
- `claudewatch correlate <outcome> [--factor <field>] [--project <name>]` CLI subcommand
- `get_causal_insights` MCP tool

---

## Suitability Assessment

**Verdict: SUITABLE**

The work decomposes cleanly into three disjoint agents:

1. **Agent A** — `internal/analyzer/correlate.go` + `internal/analyzer/correlate_test.go`: the pure analytics engine. Pearson coefficients, group comparisons, confidence flags.
2. **Agent B** — `internal/app/correlate.go`: the Cobra CLI subcommand (`claudewatch correlate`). Depends on Agent A's analyzer types.
3. **Agent C** — `internal/mcp/correlate_tools.go` + `internal/mcp/correlate_tools_test.go`: the MCP tool (`get_causal_insights`). Depends on Agent A's analyzer types.

Agents B and C both depend on Agent A's output types, so this is a Wave 1 (Agent A) → Wave 2 (Agents B and C) structure. Agents B and C are fully independent of each other.

No root-cause investigation is needed; the approach is fully specified (Pearson correlation + grouped comparisons over existing `session_meta` + `facets` data). Interface contracts are fully derivable from the codebase before any agent starts.

**Pre-implementation scan results:**
- Total items: 2 deliverables (CLI subcommand, MCP tool)
- Already implemented: 0 items
- Partially implemented: 0 items
- To-do: 2 items

All agents proceed as planned.

**Estimated times:**
- Scout phase: ~10 min (this doc)
- Agent execution: ~35 min (Agent A: ~15 min; Agents B+C: ~10 min each, parallel)
- Merge & verification: ~5 min
- Total SAW time: ~50 min

- Sequential baseline: ~45 min (15 + 15 + 15 sequential)
- Time savings: marginal on wall clock, but IMPL doc provides coordination + interface spec value regardless.

**Recommendation:** Clear speedup. Proceed. Agent A completes first; B and C then run in parallel.

---

## Known Issues

None identified. The test suite passes cleanly (`go test ./...`). The `mean` helper exists in `internal/analyzer/anomaly.go` and is unexported — Agent A must define its own `pearsonCorrelation` helper or redeclare `mean` locally (see Interface Contracts).

---

## Dependency Graph

```
session_meta (claude.SessionMeta) ─┐
facets       (claude.SessionFacet) ─┤
                                    ▼
                         [Agent A] internal/analyzer/correlate.go
                           CorrelateFactors() → FactorAnalysisReport
                                    │
                    ┌───────────────┴───────────────┐
                    ▼                               ▼
       [Agent B] internal/app/correlate.go    [Agent C] internal/mcp/correlate_tools.go
       Cobra "correlate" subcommand           MCP "get_causal_insights" handler
```

**Roots (must exist before downstream):**
- `internal/analyzer/correlate.go` — Agent A. All exported types and functions live here.
- `internal/analyzer/correlate_test.go` — Agent A.

**Leaves (no downstream dependencies within this feature):**
- `internal/app/correlate.go` — Agent B.
- `internal/mcp/correlate_tools.go` — Agent C.
- `internal/mcp/correlate_tools_test.go` — Agent C.

**Cascade candidates (files outside agent scope that reference changed interfaces):**
- `internal/mcp/tools.go` — Agent C will call `addCorrelateTools(s)` from this file. The orchestrator adds this single line to `addTools()` at the post-merge step (see Wave Execution Loop). No agent touches this file.
- `internal/app/root.go` — Agent B's `init()` registers its Cobra command on `rootCmd`; this is the existing pattern (no change to `root.go` needed — the `init()` function in Agent B's file self-registers).

---

## Interface Contracts

All signatures are binding. Agents B and C implement against these exactly as written.

### Agent A must implement

```go
// Package analyzer — file: internal/analyzer/correlate.go

// OutcomeField enumerates the supported outcome dimensions for correlation analysis.
type OutcomeField string

const (
    OutcomeFriction    OutcomeField = "friction"      // total friction events per session
    OutcomeCommits     OutcomeField = "commits"       // git commits per session
    OutcomeZeroCommit  OutcomeField = "zero_commit"   // boolean: 0 commits (1.0 or 0.0)
    OutcomeCost        OutcomeField = "cost"          // estimated cost USD
    OutcomeDuration    OutcomeField = "duration"      // session duration minutes
    OutcomeToolErrors  OutcomeField = "tool_errors"   // ToolErrors field from session meta
)

// FactorField enumerates the supported predictor (factor) dimensions.
type FactorField string

const (
    FactorHasClaudeMD  FactorField = "has_claude_md"   // bool: CLAUDE.md present in project
    FactorUsesTaskAgent FactorField = "uses_task_agent" // bool: uses_task_agent flag
    FactorUsesMCP       FactorField = "uses_mcp"        // bool: uses_mcp flag
    FactorUsesWebSearch FactorField = "uses_web_search" // bool: uses_web_search flag
    FactorIsSAW         FactorField = "is_saw"          // bool: session used SAW workflow
    FactorToolCallCount FactorField = "tool_call_count" // numeric: total tool calls
    FactorDuration      FactorField = "duration"        // numeric: session duration minutes
    FactorInputTokens   FactorField = "input_tokens"    // numeric: input tokens
)

// GroupComparison holds grouped comparison statistics for a boolean factor.
// Used for categorical factors (e.g., has_claude_md: true vs false).
type GroupComparison struct {
    // Factor is the factor field name.
    Factor FactorField `json:"factor"`

    // TrueGroup holds stats for sessions where the factor is true/positive.
    TrueGroup GroupStats `json:"true_group"`

    // FalseGroup holds stats for sessions where the factor is false/negative.
    FalseGroup GroupStats `json:"false_group"`

    // Delta is TrueGroup.AvgOutcome - FalseGroup.AvgOutcome.
    // Negative delta means the factor is associated with lower outcome values.
    Delta float64 `json:"delta"`

    // LowConfidence is true when either group has fewer than 10 sessions.
    LowConfidence bool `json:"low_confidence"`

    // Note is a human-readable interpretation, e.g. "has_claude_md sessions have
    // 0.4 lower avg friction (n=12 vs n=8, low-confidence)".
    Note string `json:"note"`
}

// GroupStats holds aggregate outcome metrics for one group.
type GroupStats struct {
    // N is the number of sessions in this group.
    N int `json:"n"`

    // AvgOutcome is the mean value of the outcome metric for this group.
    AvgOutcome float64 `json:"avg_outcome"`

    // StdDev is the standard deviation of the outcome metric.
    StdDev float64 `json:"std_dev"`
}

// PearsonResult holds the Pearson correlation coefficient between a numeric
// factor and the outcome metric.
type PearsonResult struct {
    // Factor is the factor field name.
    Factor FactorField `json:"factor"`

    // R is the Pearson correlation coefficient (-1 to 1).
    R float64 `json:"r"`

    // N is the number of sessions used in the calculation.
    N int `json:"n"`

    // LowConfidence is true when N < 10.
    LowConfidence bool `json:"low_confidence"`

    // Note is a human-readable interpretation.
    Note string `json:"note"`
}

// FactorAnalysisReport is the top-level result of a factor analysis run.
type FactorAnalysisReport struct {
    // Outcome is the outcome metric that was analyzed.
    Outcome OutcomeField `json:"outcome"`

    // Project is the project name, or empty string for all-projects analysis.
    Project string `json:"project"`

    // TotalSessions is the number of sessions included in the analysis.
    TotalSessions int `json:"total_sessions"`

    // GroupComparisons holds results for boolean factors.
    // Only populated for boolean factors when Factor arg is "" (all factors).
    GroupComparisons []GroupComparison `json:"group_comparisons,omitempty"`

    // PearsonResults holds correlation results for numeric factors.
    // Only populated when Factor arg is "" (all factors).
    PearsonResults []PearsonResult `json:"pearson_results,omitempty"`

    // SingleFactor is populated when a specific --factor was requested.
    // It holds either a GroupComparison or a PearsonResult (one will be nil).
    SingleGroupComparison *GroupComparison `json:"single_group_comparison,omitempty"`
    SinglePearson         *PearsonResult   `json:"single_pearson,omitempty"`

    // Summary is a human-readable paragraph summarizing the strongest findings.
    // Flagged low-confidence findings are noted inline.
    Summary string `json:"summary"`
}

// CorrelateInput bundles all data required for factor analysis.
// All fields except SAWSessions are required; SAWSessions may be nil
// (treated as empty — no SAW sessions in corpus).
type CorrelateInput struct {
    Sessions    []claude.SessionMeta
    Facets      []claude.SessionFacet
    SAWSessions map[string]bool // sessionID -> true for SAW sessions; nil = no SAW sessions
    ProjectPath map[string]string // sessionID -> absolute project path (for has_claude_md lookup)
    Pricing     ModelPricing
    CacheRatio  CacheRatio

    // Filters (applied before analysis):
    Project string // if non-empty, restrict to sessions whose project base name matches
    Outcome OutcomeField
    Factor  FactorField // if empty, analyze all supported factors
}

// CorrelateFactors computes factor analysis for the given input.
// It is a pure function: no I/O, no side effects.
// Returns an error if Outcome is unrecognized, or if fewer than 3 sessions remain after filtering.
func CorrelateFactors(input CorrelateInput) (FactorAnalysisReport, error)
```

### Agent B may call (from Agent A)

```go
// All types and CorrelateFactors defined in internal/analyzer/correlate.go above.
analyzer.CorrelateFactors(input analyzer.CorrelateInput) (analyzer.FactorAnalysisReport, error)
```

Agent B also uses existing functions:
```go
claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
claude.ParseAllFacets(claudeHome string) ([]claude.SessionFacet, error)
claude.ParseSessionTranscripts(claudeHome string) ([]claude.SessionSpan, error)
claude.ComputeSAWWaves(spans []claude.SessionSpan) []claude.SAWSession
claude.ParseStatsCache(claudeHome string) (*claude.StatsCache, error)
analyzer.DefaultPricing["sonnet"]          // ModelPricing
analyzer.NoCacheRatio()                    // CacheRatio
analyzer.ComputeCacheRatio(sc claude.StatsCache) CacheRatio
config.Load(flagConfig string) (*config.Config, error)   // cfg.ClaudeHome
```

### Agent C may call (from Agent A)

Same as Agent B. Agent C additionally uses the MCP server infrastructure:
```go
// Internal to mcp package — existing pattern:
func addCorrelateTools(s *Server)  // Agent C defines this and calls it from tools.go
```

---

## File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `internal/analyzer/correlate.go` | A | 1 | (existing analyzer helpers) |
| `internal/analyzer/correlate_test.go` | A | 1 | Agent A's correlate.go |
| `internal/app/correlate.go` | B | 2 | Agent A completes |
| `internal/mcp/correlate_tools.go` | C | 2 | Agent A completes |
| `internal/mcp/correlate_tools_test.go` | C | 2 | Agent C's correlate_tools.go |
| `internal/mcp/tools.go` | Orchestrator | post-merge | Agents B+C complete |

**Orchestrator-only file:** `internal/mcp/tools.go` — after all agents merge, the orchestrator adds one line to `addTools()`:
```go
addCorrelateTools(s)
```
No agent touches this file.

---

## Wave Structure

```
Wave 1: [A]          <- analyzer engine; gates all downstream
             | (A completes + Wave 1 verification passes)
Wave 2: [B] [C]      <- CLI and MCP tool, parallel
```

Unblocked by: Agent A delivering `CorrelateFactors()` and all associated types.

---

## Agent Prompts

---

### Wave 1 Agent A: Factor analysis engine

You are Wave 1 Agent A. Implement the pure analytics engine for factor analysis in `internal/analyzer/correlate.go` and its tests.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
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

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately (do NOT modify files).

**If verification passes:** Document briefly in completion report, then proceed.

## 1. File Ownership

**I1: Disjoint File Ownership.** You own only these files:
- `internal/analyzer/correlate.go` — create
- `internal/analyzer/correlate_test.go` — create

Do not touch any other files.

## 2. Interfaces You Must Implement

Implement exactly the following types and function (see Interface Contracts in IMPL doc):

```go
type OutcomeField string
const (
    OutcomeFriction   OutcomeField = "friction"
    OutcomeCommits    OutcomeField = "commits"
    OutcomeZeroCommit OutcomeField = "zero_commit"
    OutcomeCost       OutcomeField = "cost"
    OutcomeDuration   OutcomeField = "duration"
    OutcomeToolErrors OutcomeField = "tool_errors"
)

type FactorField string
const (
    FactorHasClaudeMD   FactorField = "has_claude_md"
    FactorUsesTaskAgent FactorField = "uses_task_agent"
    FactorUsesMCP       FactorField = "uses_mcp"
    FactorUsesWebSearch FactorField = "uses_web_search"
    FactorIsSAW         FactorField = "is_saw"
    FactorToolCallCount FactorField = "tool_call_count"
    FactorDuration      FactorField = "duration"
    FactorInputTokens   FactorField = "input_tokens"
)

type GroupStats struct { N int; AvgOutcome float64; StdDev float64 }
type GroupComparison struct { Factor FactorField; TrueGroup, FalseGroup GroupStats; Delta float64; LowConfidence bool; Note string }
type PearsonResult struct { Factor FactorField; R float64; N int; LowConfidence bool; Note string }
type FactorAnalysisReport struct { Outcome OutcomeField; Project string; TotalSessions int; GroupComparisons []GroupComparison; PearsonResults []PearsonResult; SingleGroupComparison *GroupComparison; SinglePearson *PearsonResult; Summary string }
type CorrelateInput struct { Sessions []claude.SessionMeta; Facets []claude.SessionFacet; SAWSessions map[string]bool; ProjectPath map[string]string; Pricing ModelPricing; CacheRatio CacheRatio; Project string; Outcome OutcomeField; Factor FactorField }

func CorrelateFactors(input CorrelateInput) (FactorAnalysisReport, error)
```

All JSON field names must match the Interface Contracts section exactly.

## 3. Interfaces You May Call

From `internal/analyzer/` (existing, unexported helpers):
- `EstimateSessionCost(sess claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64` — in `cost.go`
- `buildFacetIndex(facets []claude.SessionFacet) map[string]claude.SessionFacet` — in `compare.go`
- `sessionFriction(sess claude.SessionMeta, facetsByID map[string]claude.SessionFacet) int` — in `compare.go`

The `mean` function in `anomaly.go` is unexported. Define your own `mean` and `stddev` helpers locally in `correlate.go` rather than duplicating the symbol. Do not modify `anomaly.go`.

## 4. What to Implement

Read `internal/analyzer/anomaly.go`, `internal/analyzer/compare.go`, and `internal/analyzer/experiment.go` to understand existing patterns before writing code.

**`CorrelateFactors` behavior:**

1. **Filter sessions by project** (if `input.Project != ""`): keep only sessions where `filepath.Base(sess.ProjectPath)` matches `input.Project`.

2. **Require minimum data**: return an error if fewer than 3 sessions remain after filtering.

3. **Compute the outcome vector**: for each session, compute the numeric outcome value:
   - `friction`: sum of all `FrictionCounts` values from the matching facet; fall back to `sess.ToolErrors` if no facet.
   - `commits`: `sess.GitCommits`.
   - `zero_commit`: `1.0` if `sess.GitCommits == 0`, else `0.0`.
   - `cost`: `EstimateSessionCost(sess, input.Pricing, input.CacheRatio)`.
   - `duration`: `float64(sess.DurationMinutes)`.
   - `tool_errors`: `float64(sess.ToolErrors)`.

4. **Boolean factors** — `has_claude_md`, `uses_task_agent`, `uses_mcp`, `uses_web_search`, `is_saw`:
   - Split sessions into TrueGroup and FalseGroup.
   - Compute `GroupStats` (N, AvgOutcome, StdDev) for each group.
   - Compute `Delta = TrueGroup.AvgOutcome - FalseGroup.AvgOutcome`.
   - Set `LowConfidence = true` if either group has N < 10.
   - Generate a one-sentence `Note` (e.g., `"has_claude_md sessions avg 0.4 friction vs 1.2 (n=14 vs n=6, low-confidence)"`).
   - For `has_claude_md`: use `input.ProjectPath[sess.SessionID]` to look up the project path, then check if `<path>/CLAUDE.md` exists via `os.Stat`. This requires `"os"` and `"path/filepath"` imports.
   - For `is_saw`: use `input.SAWSessions[sess.SessionID]`.

5. **Numeric factors** — `tool_call_count`, `duration`, `input_tokens`:
   - Compute Pearson correlation coefficient between the factor vector and the outcome vector.
   - Pearson formula: `r = Σ((xi - x̄)(yi - ȳ)) / sqrt(Σ(xi - x̄)² · Σ(yi - ȳ)²)`.
   - If denominator is 0 (no variance), return `r = 0`.
   - Set `LowConfidence = true` if N < 10.
   - Generate a `Note` (e.g., `"tool_call_count has r=0.62 correlation with friction (n=20)"`).
   - For `tool_call_count`: sum all values in `sess.ToolCounts`.
   - For `duration`: `sess.DurationMinutes`.
   - For `input_tokens`: `sess.InputTokens`.

6. **Single factor mode** (when `input.Factor != ""`): compute only that one factor. Populate `SingleGroupComparison` or `SinglePearson` as appropriate; leave `GroupComparisons` and `PearsonResults` nil.

7. **All-factors mode** (when `input.Factor == ""`): compute all boolean factors → `GroupComparisons`; compute all numeric factors → `PearsonResults`.

8. **Summary**: generate a multi-sentence plain English paragraph:
   - For group comparisons: highlight the factor with the largest absolute delta.
   - For Pearson results: highlight the factor with the largest |r|.
   - Always include n values and flag low-confidence findings.
   - Example: `"has_claude_md is the strongest factor: sessions with CLAUDE.md average 0.4 friction vs 1.2 without (n=14 vs n=6, low-confidence). tool_call_count shows moderate positive correlation with friction (r=0.52, n=20)."`

9. **Populate TotalSessions** from the count of sessions after project filtering.

**Error handling:**
- Return `(FactorAnalysisReport{}, error)` if `input.Outcome` is not one of the six recognized values.
- Return `(FactorAnalysisReport{}, error)` if `input.Factor != ""` and the factor name is not recognized.
- Return `(FactorAnalysisReport{}, error)` if fewer than 3 sessions remain.

## 5. Tests to Write

1. `TestCorrelateFactors_AllFactors_BasicSmoke` — build a corpus of 15 synthetic sessions with known properties (half with CLAUDE.md, half without; varying friction); verify `GroupComparisons` is populated, `has_claude_md` has correct Delta sign, `TotalSessions == 15`.
2. `TestCorrelateFactors_SingleFactor_Boolean` — `Factor = FactorHasClaudeMD`; verify `SingleGroupComparison` is non-nil, `GroupComparisons` is nil.
3. `TestCorrelateFactors_SingleFactor_Numeric` — `Factor = FactorToolCallCount`; verify `SinglePearson` is non-nil, `PearsonResults` is nil.
4. `TestCorrelateFactors_LowConfidenceFlagged` — corpus of 8 sessions total; verify at least one `GroupComparison.LowConfidence == true`.
5. `TestCorrelateFactors_ProjectFilter` — corpus with two projects; filter to one; verify `TotalSessions` matches only filtered sessions.
6. `TestCorrelateFactors_InsufficientData` — 2 sessions; verify error returned.
7. `TestCorrelateFactors_UnknownOutcome` — `Outcome = "invalid"`; verify error returned.
8. `TestPearsonCorrelation_PerfectPositive` — x = [1,2,3,4,5], y = [2,4,6,8,10]; verify r ≈ 1.0.
9. `TestPearsonCorrelation_NoVariance` — x = [3,3,3], y = [1,2,3]; verify r = 0 (no panic).
10. `TestCorrelateFactors_ZeroCommitOutcome` — sessions with mix of 0 and non-0 commits; verify `OutcomeZeroCommit` produces values of 0.0 or 1.0 only.

For synthetic sessions, set `has_claude_md` via the `ProjectPath` field and a temp directory with/without a `CLAUDE.md` file. Alternatively, provide a `ProjectPath` map with pre-seeded values.

Note: `has_claude_md` requires `os.Stat`. In tests, use `t.TempDir()` to create real temp directories with and without `CLAUDE.md` files, and populate `input.ProjectPath` accordingly.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/analyzer -run TestCorrelate -v -timeout 60s
go test ./internal/analyzer -run TestPearson -v -timeout 60s
```

All must pass before reporting completion.

## 7. Constraints

- `CorrelateFactors` is a pure function. No I/O except `os.Stat` for `has_claude_md` lookups (which uses the provided `ProjectPath` map — the caller supplies the path, the analyzer does the stat).
- Do not import `store` package. This feature does not use the SQLite DB.
- Use `math` for Pearson; no third-party imports.
- JSON field names must exactly match the Interface Contracts. Downstream agents implement against these tags.
- `GroupComparisons` and `PearsonResults` must be `nil` (not `[]GroupComparison{}`) when not applicable to avoid confusing MCP consumers.
- `LowConfidence` threshold is N < 10 per group for boolean factors; N < 10 total for Pearson.
- If you discover that correct implementation requires changing a file outside your ownership, do NOT modify it. Report it in section 8.

## 8. Report

**I5: Agents Commit Before Reporting.** Commit before writing report.

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
git add internal/analyzer/correlate.go internal/analyzer/correlate_test.go
git commit -m "wave1-agent-a: factor analysis engine (CorrelateFactors)"
```

```yaml
### Agent A - Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-a
commit: {sha}
files_changed: []
files_created:
  - internal/analyzer/correlate.go
  - internal/analyzer/correlate_test.go
interface_deviations:
  - "[] or list any deviation from Interface Contracts"
out_of_scope_deps:
  - "[] or list any file outside ownership that needs a change"
tests_added:
  - TestCorrelateFactors_AllFactors_BasicSmoke
  - TestCorrelateFactors_SingleFactor_Boolean
  - TestCorrelateFactors_SingleFactor_Numeric
  - TestCorrelateFactors_LowConfidenceFlagged
  - TestCorrelateFactors_ProjectFilter
  - TestCorrelateFactors_InsufficientData
  - TestCorrelateFactors_UnknownOutcome
  - TestPearsonCorrelation_PerfectPositive
  - TestPearsonCorrelation_NoVariance
  - TestCorrelateFactors_ZeroCommitOutcome
verification: PASS | FAIL ({command} - N/N tests)
```

---

### Wave 2 Agent B: `claudewatch correlate` CLI subcommand

You are Wave 2 Agent B. Implement the `claudewatch correlate` Cobra subcommand in `internal/app/correlate.go`.

**Prerequisite:** Agent A (Wave 1) must have completed and been merged before you start. The types `analyzer.CorrelateFactors`, `analyzer.CorrelateInput`, `analyzer.FactorAnalysisReport`, `analyzer.OutcomeField`, `analyzer.FactorField`, and all associated constants must exist in `internal/analyzer/correlate.go`.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-b"

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

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own only:
- `internal/app/correlate.go` — create

Do not touch any other files. The `init()` function in your file self-registers on `rootCmd`; no change to `root.go` is needed.

## 2. Interfaces You Must Implement

A Cobra command registered as `correlate` on `rootCmd`:

```
claudewatch correlate <outcome> [--factor <field>] [--project <name>]
```

- Positional arg: `<outcome>` — one of `friction`, `commits`, `zero_commit`, `cost`, `duration`, `tool_errors`.
- `--factor <field>` (optional) — one of `has_claude_md`, `uses_task_agent`, `uses_mcp`, `uses_web_search`, `is_saw`, `tool_call_count`, `duration`, `input_tokens`.
- `--project <name>` (optional) — filter to a single project by base name.
- `--json` (global flag, already defined in `root.go`) — output JSON.

## 3. Interfaces You May Call

```go
// From Agent A (internal/analyzer/correlate.go):
analyzer.CorrelateFactors(input analyzer.CorrelateInput) (analyzer.FactorAnalysisReport, error)
// All OutcomeField and FactorField constants.

// Existing functions:
claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
claude.ParseAllFacets(claudeHome string) ([]claude.SessionFacet, error)
claude.ParseSessionTranscripts(claudeHome string) ([]claude.SessionSpan, error)
claude.ComputeSAWWaves(spans []claude.SessionSpan) []claude.SAWSession
claude.ParseStatsCache(claudeHome string) (*claude.StatsCache, error)
analyzer.DefaultPricing["sonnet"]
analyzer.NoCacheRatio()
analyzer.ComputeCacheRatio(sc claude.StatsCache) analyzer.CacheRatio
config.Load(flagConfig string) (*config.Config, error)
output.Section(title string) string
output.NewTable(cols ...string) *output.Table
output.StyleBold, output.StyleMuted, output.StyleLabel, output.StyleValue, output.StyleWarning
```

Read `internal/app/compare.go` as a structural reference for how a CLI command loads data and renders results.

## 4. What to Implement

**Command registration** (follow the pattern from `compare.go`, `sessions.go`):

```go
var correlateCmd = &cobra.Command{
    Use:   "correlate <outcome>",
    Short: "Correlate session attributes against outcomes",
    Long: `...`,
    Args: cobra.ExactArgs(1),
    RunE: runCorrelate,
}

func init() {
    correlateCmd.Flags().StringVar(&correlateFlagFactor, "factor", "", "Factor field to analyze (default: all)")
    correlateCmd.Flags().StringVar(&correlateFlagProject, "project", "", "Filter to a specific project by name")
    rootCmd.AddCommand(correlateCmd)
}
```

**Data loading in `runCorrelate`:**

1. Load sessions via `claude.ParseAllSessionMeta`.
2. Load facets via `claude.ParseAllFacets`.
3. Load SAW sessions: parse transcripts via `claude.ParseSessionTranscripts`, compute SAW waves via `claude.ComputeSAWWaves`, build `sawSessionIDs map[string]bool`.
4. Build `projectPathMap map[string]string` (sessionID → sess.ProjectPath) for the `has_claude_md` check.
5. Load cache ratio (non-fatal).
6. Build `analyzer.CorrelateInput` and call `analyzer.CorrelateFactors`.

**Rendering (non-JSON):**

When `--factor` is omitted (all factors):
- Print a section header: `"Factor Analysis: <outcome>"`.
- Print a table with columns `"Factor"`, `"True Group (n)"`, `"Avg"`, `"False Group (n)"`, `"Avg"`, `"Delta"`, `"Confidence"`.
  - One row per `GroupComparison`.
  - Confidence column: `"low"` (styled with `output.StyleWarning`) or `""`.
- Print a Pearson table with columns `"Factor"`, `"r"`, `"n"`, `"Confidence"`.
  - One row per `PearsonResult`.
- Print the `Summary` paragraph.

When `--factor` is specified:
- Print a focused single-factor view (no table; just labeled key-value lines).
- Boolean factor: show TrueGroup N+avg, FalseGroup N+avg, delta, confidence, note.
- Numeric factor: show r, n, confidence, note.

**JSON output:** marshal `FactorAnalysisReport` directly via `json.NewEncoder(os.Stdout).Encode(report)`.

**Error handling:**
- If `<outcome>` is not one of the recognized values, return a helpful error: `"unrecognized outcome %q; valid values: friction, commits, zero_commit, cost, duration, tool_errors"`.
- If `--factor` is not a recognized value, return a helpful error.
- If `CorrelateFactors` returns an error (e.g., insufficient data), print a user-friendly message and return nil (not a fatal error).

## 5. Tests to Write

No unit tests required for this file (it is a thin CLI layer over the analyzer). If you write tests, place them in `internal/app/correlate_test.go` (which you may create and will also own).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b
go build ./...
go vet ./...
go test ./internal/app -run TestCorrelate -v -timeout 60s
```

If no `_test.go` was created, the test command will trivially pass. The build must succeed.

**Manual smoke test (optional but recommended):**
```bash
./bin/claudewatch correlate friction
./bin/claudewatch correlate friction --factor has_claude_md
./bin/claudewatch correlate commits --project claudewatch --json
```

## 7. Constraints

- Do not modify `root.go` or any file outside your ownership.
- Non-fatal errors (insufficient data, no sessions) should print a friendly message rather than returning a CLI error.
- The `--json` flag is a global flag defined in `root.go`; reference it as `flagJSON` (same as other commands).
- The `--no-color` flag is a global flag; check `flagNoColor` and call `output.SetNoColor(true)` if set, same as other commands.
- Do not import the `store` package.

## 8. Report

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b
git add internal/app/correlate.go
git commit -m "wave2-agent-b: claudewatch correlate CLI subcommand"
```

```yaml
### Agent B - Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-b
commit: {sha}
files_changed: []
files_created:
  - internal/app/correlate.go
interface_deviations:
  - "[] or list any deviation"
out_of_scope_deps:
  - "[] or list any file outside ownership that needs a change"
tests_added: []
verification: PASS | FAIL ({command})
```

---

### Wave 2 Agent C: `get_causal_insights` MCP tool

You are Wave 2 Agent C. Implement the `get_causal_insights` MCP tool in `internal/mcp/correlate_tools.go` and its tests.

**Prerequisite:** Agent A (Wave 1) must have completed and been merged. The types and `CorrelateFactors` function must exist in `internal/analyzer/correlate.go`.

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

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own only:
- `internal/mcp/correlate_tools.go` — create
- `internal/mcp/correlate_tools_test.go` — create

Do not touch `internal/mcp/tools.go`. The orchestrator will add `addCorrelateTools(s)` to that file after all agents merge.

## 2. Interfaces You Must Implement

```go
// In internal/mcp/correlate_tools.go:

// CausalInsightsResult is the JSON response from get_causal_insights.
// It wraps analyzer.FactorAnalysisReport directly (embed or mirror).
type CausalInsightsResult struct {
    Outcome       string                     `json:"outcome"`
    Project       string                     `json:"project,omitempty"`
    TotalSessions int                        `json:"total_sessions"`
    GroupComparisons []GroupComparisonResult `json:"group_comparisons,omitempty"`
    PearsonResults   []PearsonResultEntry    `json:"pearson_results,omitempty"`
    SingleGroupComparison *GroupComparisonResult `json:"single_group_comparison,omitempty"`
    SinglePearson         *PearsonResultEntry    `json:"single_pearson,omitempty"`
    Summary       string                     `json:"summary"`
}

type GroupStatsResult struct {
    N          int     `json:"n"`
    AvgOutcome float64 `json:"avg_outcome"`
    StdDev     float64 `json:"std_dev"`
}

type GroupComparisonResult struct {
    Factor        string           `json:"factor"`
    TrueGroup     GroupStatsResult `json:"true_group"`
    FalseGroup    GroupStatsResult `json:"false_group"`
    Delta         float64          `json:"delta"`
    LowConfidence bool             `json:"low_confidence"`
    Note          string           `json:"note"`
}

type PearsonResultEntry struct {
    Factor        string  `json:"factor"`
    R             float64 `json:"r"`
    N             int     `json:"n"`
    LowConfidence bool    `json:"low_confidence"`
    Note          string  `json:"note"`
}

// addCorrelateTools registers the get_causal_insights tool on s.
// The orchestrator calls this from tools.go after all agents merge.
func addCorrelateTools(s *Server)

// handleGetCausalInsights is the MCP handler for get_causal_insights.
func (s *Server) handleGetCausalInsights(args json.RawMessage) (any, error)
```

## 3. Interfaces You May Call

```go
// From Agent A (internal/analyzer/correlate.go):
analyzer.CorrelateFactors(input analyzer.CorrelateInput) (analyzer.FactorAnalysisReport, error)
// All OutcomeField and FactorField constants.

// Existing MCP-package helpers:
s.loadTags() map[string]string
resolveProjectName(sessionID, projectPath string, tags map[string]string) string
s.loadCacheRatio() analyzer.CacheRatio

// Existing data parsers:
claude.ParseAllSessionMeta(s.claudeHome) ([]claude.SessionMeta, error)
claude.ParseAllFacets(s.claudeHome) ([]claude.SessionFacet, error)
claude.ParseSessionTranscripts(s.claudeHome) ([]claude.SessionSpan, error)
claude.ComputeSAWWaves(spans []claude.SessionSpan) []claude.SAWSession
analyzer.DefaultPricing["sonnet"]
```

Read `internal/mcp/health_tools.go` and `internal/mcp/regression_tools.go` as structural references.

## 4. What to Implement

**Tool registration in `addCorrelateTools`:**

```go
func addCorrelateTools(s *Server) {
    s.registerTool(toolDef{
        Name: "get_causal_insights",
        Description: "Correlate session attributes against outcomes to identify what factors predict good sessions. Supports outcomes: friction, commits, zero_commit, cost, duration, tool_errors. Supports factors: has_claude_md, uses_task_agent, uses_mcp, uses_web_search, is_saw, tool_call_count, duration, input_tokens. Groups with n < 10 are flagged as low-confidence.",
        InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "outcome": {
                    "type": "string",
                    "description": "Outcome metric to analyze: friction, commits, zero_commit, cost, duration, tool_errors",
                    "enum": ["friction","commits","zero_commit","cost","duration","tool_errors"]
                },
                "factor": {
                    "type": "string",
                    "description": "Optional: specific factor to analyze. If omitted, all factors are analyzed.",
                    "enum": ["has_claude_md","uses_task_agent","uses_mcp","uses_web_search","is_saw","tool_call_count","duration","input_tokens"]
                },
                "project": {
                    "type": "string",
                    "description": "Optional: filter to a specific project by name."
                }
            },
            "required": ["outcome"],
            "additionalProperties": false
        }`),
        Handler: s.handleGetCausalInsights,
    })
}
```

**Handler `handleGetCausalInsights`:**

1. Parse `outcome` (required), `factor` (optional), `project` (optional) from args.
2. Return a JSON error if `outcome` is missing or unrecognized.
3. Load sessions, facets, SAW sessions (non-fatal on SAW parse failure — treat as empty).
4. Build `projectPathMap map[string]string` (sessionID → sess.ProjectPath).
5. Build `analyzer.CorrelateInput` and call `analyzer.CorrelateFactors`.
6. If `CorrelateFactors` returns an error (e.g., insufficient data), return the error to the MCP client (non-nil error → MCP will return it as an error response).
7. Map the `analyzer.FactorAnalysisReport` to `CausalInsightsResult` and return it.

**Mapping from `analyzer.FactorAnalysisReport` to `CausalInsightsResult`:**

The MCP result type mirrors the analyzer type. Convert by mapping each field. The `string(field)` conversion handles `OutcomeField` → `string` and `FactorField` → `string`.

## 5. Tests to Write

Write tests in `internal/mcp/correlate_tools_test.go`.

1. `TestHandleGetCausalInsights_AllFactors` — set up a temp `claudeHome` with 15 synthetic session-meta JSON files and matching facets; call `handleGetCausalInsights` with `{"outcome":"friction"}`; verify `total_sessions` > 0 and `group_comparisons` is non-empty.
2. `TestHandleGetCausalInsights_SingleFactor` — call with `{"outcome":"friction","factor":"has_claude_md"}`; verify `single_group_comparison` is non-nil and `group_comparisons` is nil.
3. `TestHandleGetCausalInsights_InsufficientData` — 2 sessions; verify handler returns an error (not a nil error).
4. `TestHandleGetCausalInsights_MissingOutcome` — call with `{}`; verify error returned.
5. `TestHandleGetCausalInsights_ProjectFilter` — sessions from two projects; filter with `{"outcome":"commits","project":"myproject"}`; verify `total_sessions` reflects only one project's sessions.

Use the `writeSessionMeta` and `writeFacet` helpers from `tools_test.go` (they are in the same `mcp` package and accessible within the test file).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
go build ./...
go vet ./...
go test ./internal/mcp -run TestHandleGetCausalInsights -v -timeout 60s
```

All must pass before reporting completion.

## 7. Constraints

- Do not modify `internal/mcp/tools.go`. The orchestrator adds `addCorrelateTools(s)` there after merge.
- Do not import the `store` package. This feature does not use SQLite.
- The `addCorrelateTools` function must be exported from the `mcp` package (lowercase `add` prefix follows the existing convention — it is package-internal, not exported to other packages, as per the existing `addAnalyticsTools`, `addCostTools` pattern).
- Non-nil errors from `CorrelateFactors` should be returned as errors (not swallowed), so the MCP client sees an error response.
- JSON field names in `CausalInsightsResult` and sub-types must match exactly as specified in section 2.

## 8. Report

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
git add internal/mcp/correlate_tools.go internal/mcp/correlate_tools_test.go
git commit -m "wave2-agent-c: get_causal_insights MCP tool"
```

```yaml
### Agent C - Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-c
commit: {sha}
files_changed: []
files_created:
  - internal/mcp/correlate_tools.go
  - internal/mcp/correlate_tools_test.go
interface_deviations:
  - "[] or list any deviation"
out_of_scope_deps:
  - "[] or list any file outside ownership that needs a change"
tests_added:
  - TestHandleGetCausalInsights_AllFactors
  - TestHandleGetCausalInsights_SingleFactor
  - TestHandleGetCausalInsights_InsufficientData
  - TestHandleGetCausalInsights_MissingOutcome
  - TestHandleGetCausalInsights_ProjectFilter
verification: PASS | FAIL ({command} - N/N tests)
```

---

## Wave Execution Loop

After Wave 1 completes:
1. Read Agent A's completion report. Check for interface deviations and out-of-scope deps.
2. Merge Agent A's worktree branch into main.
3. Run full verification gate:
   ```bash
   go build ./...
   go vet ./...
   go test ./internal/analyzer -run TestCorrelate -timeout 60s
   go test ./internal/analyzer -run TestPearson -timeout 60s
   ```
4. Fix any issues.
5. Tick Agent A status below.
6. Launch Wave 2 (Agents B and C in parallel).

After Wave 2 completes:
1. Read Agent B and C completion reports.
2. Merge both worktree branches into main (B first, then C, or use `git merge`).
3. Add `addCorrelateTools(s)` to `internal/mcp/tools.go` inside the `addTools` function, following the existing pattern (`addAnalyticsTools(s)`, etc.).
4. Run linter auto-fix (orchestrator responsibility):
   ```bash
   gofmt -w .
   golangci-lint run --fix ./...
   ```
5. Run full verification gate:
   ```bash
   go build ./...
   go vet ./...
   go test ./... -timeout 120s
   ```
6. Fix any integration issues.
7. Commit the merged and fixed result.

---

## Status

- [x] Wave 1 Agent A — factor analysis engine (`internal/analyzer/correlate.go`, `correlate_test.go`)
- [ ] Wave 2 Agent B — `claudewatch correlate` CLI subcommand (`internal/app/correlate.go`)
- [ ] Wave 2 Agent C — `get_causal_insights` MCP tool (`internal/mcp/correlate_tools.go`, `correlate_tools_test.go`)
- [ ] Orchestrator post-merge — add `addCorrelateTools(s)` to `internal/mcp/tools.go`

---

### Agent A - Completion Report
status: complete
worktree: main (solo agent)
commit: c0d5af3
files_changed: []
files_created:
  - internal/analyzer/correlate.go
  - internal/analyzer/correlate_test.go
interface_deviations:
  - []
out_of_scope_deps:
  - []
tests_added:
  - TestCorrelateFactors_AllFactors_BasicSmoke
  - TestCorrelateFactors_SingleFactor_Boolean
  - TestCorrelateFactors_SingleFactor_Numeric
  - TestCorrelateFactors_LowConfidenceFlagged
  - TestCorrelateFactors_ProjectFilter
  - TestCorrelateFactors_InsufficientData
  - TestCorrelateFactors_UnknownOutcome
  - TestPearsonCorrelation_PerfectPositive
  - TestPearsonCorrelation_NoVariance
  - TestCorrelateFactors_ZeroCommitOutcome
verification: PASS (go test ./internal/analyzer -run TestCorrelate -v - 8/8 tests; go test ./internal/analyzer -run TestPearson -v - 2/2 tests)

### Agent B - Completion Report
status: complete
worktree: .claude/worktrees/wave2-agent-b
commit: 093100e999284a1d1eec5f4286be50b50e7a8e79
files_changed: []
files_created:
  - internal/app/correlate.go
interface_deviations:
  - []
out_of_scope_deps:
  - []
tests_added: []
verification: PASS (go build ./...; go vet ./...)

### Agent C - Completion Report
status: complete
worktree: .claude/worktrees/wave2-agent-c
commit: 01174ea
files_changed: []
files_created:
  - internal/mcp/correlate_tools.go
  - internal/mcp/correlate_tools_test.go
interface_deviations:
  - []
out_of_scope_deps:
  - []
tests_added:
  - TestHandleGetCausalInsights_AllFactors
  - TestHandleGetCausalInsights_SingleFactor
  - TestHandleGetCausalInsights_InsufficientData
  - TestHandleGetCausalInsights_MissingOutcome
  - TestHandleGetCausalInsights_ProjectFilter
verification: PASS (go test ./internal/mcp -run TestHandleGetCausalInsights -v -timeout 60s - 5/5 tests)
