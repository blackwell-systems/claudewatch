# IMPL: Sessions Enhancements

Four features: accurate cost estimation, session summary stats, session inspect, and doctor command.

## Suitability Assessment

**Verdict: SUITABLE**

The work decomposes into 3 agents across 2 waves with fully disjoint file ownership. No investigation-first items — all data sources and patterns are well-understood. Cross-agent interfaces are a single exported function (`EstimateSessionCost`) whose signature can be defined before implementation starts.

The dependency structure is:
- Wave 1 runs two independent agents in parallel: Agent A (export cost function + tests) and Agent B (doctor command, entirely new files).
- Wave 2 runs Agent C (sessions enhancements: wire cost, add inspect, add summary stats, wire cost into watcher).

Agent A must complete before Agent C because C depends on the exported `EstimateSessionCost` function.

**Pre-implementation status:** All 4 features are TO-DO (none partially implemented).

## Known Issues

None identified.

## Dependency Graph

```
analyzer/outcomes.go (export estimateSessionCost)
    ↓
app/sessions.go (wire cost, add --inspect, add summary stats)
app/doctor.go (independent — new file)
watcher/watcher.go (wire cost)
```

Root: `analyzer/outcomes.go` — the exported function gates downstream work.
Leaves: `app/sessions.go`, `app/doctor.go`, `watcher/watcher.go`.

**Cascade candidates** (files NOT changed but referencing changed interfaces):
- `internal/analyzer/effectiveness.go` — calls `estimateSessionCost` (lowercase). When renamed to exported, this file must update its call site. Agent A owns this.
- `internal/analyzer/outcomes.go` — contains both the function definition and internal callers. Agent A owns this.

## Interface Contracts

### Exported by Agent A (Wave 1)

```go
// In package analyzer (internal/analyzer/outcomes.go)

// EstimateSessionCost computes the dollar cost of a single session from its
// token counts using the given pricing and cache ratio.
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64
```

This is an export of the existing `estimateSessionCost` function — same signature, same behavior, just capitalized. Internal callers in `outcomes.go` and `effectiveness.go` must be updated to call `EstimateSessionCost`.

### No new interfaces from Agent B or C

Agent B (doctor) creates a standalone command with no shared interfaces.
Agent C consumes `EstimateSessionCost` but introduces no new cross-agent interfaces.

## File Ownership

| File | Agent | Wave | Action | Depends On |
|------|-------|------|--------|------------|
| `internal/analyzer/outcomes.go` | A | 1 | modify (export function) | — |
| `internal/analyzer/outcomes_test.go` | A | 1 | modify (add export test) | — |
| `internal/analyzer/effectiveness.go` | A | 1 | modify (update call site) | — |
| `internal/app/doctor.go` | B | 1 | create | — |
| `internal/app/sessions.go` | C | 2 | modify | Agent A |
| `internal/watcher/watcher.go` | C | 2 | modify (wire cost) | Agent A |

## Wave Structure

```
Wave 1: [A] [B]    <- 2 parallel agents
           |
Wave 2:   [C]      <- 1 agent (depends on A)
```

## Agent Prompts

### Wave 1 Agent A: Export session cost function

You are Wave 1 Agent A. Export the existing `estimateSessionCost` function in the analyzer package so it can be called from other packages.

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/analyzer/outcomes.go` — modify
- `internal/analyzer/outcomes_test.go` — modify
- `internal/analyzer/effectiveness.go` — modify

#### 2. Interfaces You Must Implement

```go
// Export the existing estimateSessionCost by capitalizing it.
// Same signature, same behavior.
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64
```

#### 3. Interfaces You May Call

All existing functions in the analyzer package. No new dependencies.

#### 4. What to Implement

1. In `internal/analyzer/outcomes.go`, rename `estimateSessionCost` to `EstimateSessionCost` (capitalize).
2. Update all call sites within `outcomes.go` that call `estimateSessionCost` to call `EstimateSessionCost`.
3. In `internal/analyzer/effectiveness.go`, update the call to `estimateSessionCost` to `EstimateSessionCost`.
4. Read the existing files first to find all call sites. Use grep to be thorough.

This is a minimal change — rename only, no logic changes.

#### 5. Tests to Write

1. `TestEstimateSessionCost_Exported` — verify the exported function returns the same values as the old unexported one (basic smoke test with known token counts and Sonnet pricing).

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/analyzer/ -run TestEstimateSessionCost -count=1
go test ./internal/analyzer/ -count=1
```

#### 7. Constraints

- Do NOT change the function's logic or signature beyond capitalizing the name.
- Do NOT add parameters or change return types.
- Ensure all existing tests still pass — this is a pure rename.

#### 8. Report

Append your completion report to the IMPL doc under `### Agent A — Completion Report`.

---

### Wave 1 Agent B: Doctor command

You are Wave 1 Agent B. Create a new `doctor` command that checks whether the user's claudewatch setup is healthy.

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/app/doctor.go` — create

#### 2. Interfaces You Must Implement

No shared interfaces. This is a standalone Cobra command.

#### 3. Interfaces You May Call

```go
// Existing functions you can use:
config.Load(path string) (*config.Config, error)
config.DBPath() string
config.ConfigDir() string
claude.ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error)
claude.ParseStatsCache(claudeHome string) (*StatsCache, error)
scanner.DiscoverProjects(scanPaths []string) ([]scanner.Project, error)
output.Section(title string) string
output.SetNoColor(b bool)
// Styles: output.StyleSuccess, output.StyleWarning, output.StyleMuted, output.StyleBold
```

#### 4. What to Implement

Create `internal/app/doctor.go` with a `doctor` Cobra command that runs these checks:

1. **Claude home directory** — `cfg.ClaudeHome` exists and is readable
2. **Session data** — at least 1 session-meta file exists
3. **Stats cache** — `stats-cache.json` exists and parses
4. **Scan paths** — each configured scan path exists
5. **SQLite database** — `config.DBPath()` exists (for `track` to work)
6. **Watch daemon** — check if PID file exists at `config.ConfigDir()/watch.pid` and if process is running
7. **CLAUDE.md coverage** — count projects with vs without CLAUDE.md
8. **API key** — `ANTHROPIC_API_KEY` env var is set (needed for `fix --ai`)

For each check, print a pass/fail line with a check mark or X. At the end, print a summary: "N/M checks passed".

Follow the same patterns as other commands in `internal/app/`:
- Register with `rootCmd.AddCommand(doctorCmd)` in `init()`
- Respect `flagNoColor` and `flagJSON`
- JSON mode outputs a struct with all check results

Read `internal/app/watch.go` for the PID file path pattern (`pidFilePath()`, `readPID()`) — but do NOT import those functions (they're unexported). Reimplement the PID check inline or with `os.ReadFile` + `strconv.Atoi` + process existence check via `os.FindProcess` + signal 0.

Read `internal/app/gaps.go` and `internal/app/scan.go` for patterns on how commands are structured.

#### 5. Tests to Write

No tests required — this is a CLI command with side effects (filesystem checks, process checks). The verification gate (build + vet) is sufficient.

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
```

#### 7. Constraints

- Do NOT modify any existing files.
- Use `os.Stat` for existence checks, not `os.Open`.
- Process existence check: `os.FindProcess(pid)` then `process.Signal(syscall.Signal(0))` — nil error means running. Import `syscall` only for this.
- Keep output compact: one line per check, summary at end.
- Respect `--json` and `--no-color` global flags.

#### 8. Report

Append your completion report to the IMPL doc under `### Agent B — Completion Report`.

---

### Wave 2 Agent C: Sessions enhancements

You are Wave 2 Agent C. Add three enhancements to the sessions command and wire accurate cost estimation into the watcher.

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/app/sessions.go` — modify
- `internal/watcher/watcher.go` — modify

#### 2. Interfaces You Must Implement

No new shared interfaces.

#### 3. Interfaces You May Call

```go
// From Wave 1 Agent A (now exported):
analyzer.EstimateSessionCost(s claude.SessionMeta, pricing analyzer.ModelPricing, ratio analyzer.CacheRatio) float64

// Existing:
analyzer.DefaultPricing    // map[string]ModelPricing — keys: "opus", "sonnet", "haiku"
analyzer.NoCacheRatio()    // CacheRatio
analyzer.ComputeCacheRatio(stats claude.StatsCache) CacheRatio
claude.ParseStatsCache(claudeHome string) (*StatsCache, error)
```

#### 4. What to Implement

**4a. Wire accurate cost in sessions.go**

Replace the inline `estimatedCost()` method on `sessionRow` with a call to `analyzer.EstimateSessionCost`. Use Sonnet pricing as default (most common model). Load stats-cache if available to get cache ratio; fall back to `NoCacheRatio()`.

Load stats-cache once in `runSessions()` (non-fatal if missing), compute the cache ratio, and pass pricing+ratio into the render pipeline. The `sessionRow` type should store a precomputed cost field rather than calling `estimatedCost()` repeatedly.

**4b. Add summary stats footer**

After the table in `renderSessions()`, add a summary line showing:
- Total cost across displayed sessions
- Average friction per session
- Total commits
- Average duration

Format: ` Totals: $X.XX cost · Y commits · Z.Z avg friction · Wm avg duration`

**4c. Add `--inspect <session-id>` mode**

Add a positional argument: `claudewatch sessions <session-id>`. When a session ID (or prefix) is provided, show a detailed single-session view instead of the table:

- Session ID, project, date, duration
- Messages: user / assistant counts
- Tokens: input / output, estimated cost
- Git: commits, pushes, lines added/removed, files modified
- Tools: top 5 tools by usage count
- Friction: each friction type and count (from facet)
- Outcome, satisfaction, goal, summary (from facet)
- First prompt (truncated to 200 chars)

Use `output.Section()` and styled labels. Match by full session ID or prefix (first 8+ chars).

Update the `Args` on `sessionsCmd` from implicit `cobra.NoArgs` to `cobra.MaximumNArgs(1)`.

**4d. Wire accurate cost in watcher.go**

Replace the inline cost calculation in `Snapshot()` (lines ~220-224) with `analyzer.EstimateSessionCost`. Load stats-cache in `Snapshot()` (non-fatal). Use Sonnet default pricing. Add necessary imports.

#### 5. Tests to Write

No new test files — the `app` package has no test files, and watcher tests exercise `Snapshot()` via mocked data that won't have stats-cache. The verification gate (build + full test suite) is sufficient.

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./... -count=1
```

#### 7. Constraints

- Use `analyzer.DefaultPricing["sonnet"]` as the default pricing tier.
- Stats-cache loading must be non-fatal — if it fails, use `analyzer.NoCacheRatio()`.
- The `--inspect` view should work even if no facet data exists for that session.
- Do NOT add new packages or create new files.
- Respect `--json` flag for inspect mode (output the full sessionRow as JSON).

#### 8. Report

Append your completion report to the IMPL doc under `### Agent C — Completion Report`.

## Wave Execution Loop

After each wave completes:
1. Read each agent's completion report from their named section in the IMPL doc.
2. Merge all agent worktrees back into the main branch.
3. Run the full verification gate:
   ```bash
   cd /Users/dayna.blackwell/code/claudewatch
   go build ./...
   go vet ./...
   go test ./... -count=1
   ```
4. Pay attention to cascade candidates: `effectiveness.go` references the renamed function.
5. Fix any compiler errors or integration issues.
6. Update this doc: tick status checkboxes, correct any interface contracts that changed.
7. Commit the wave's changes.
8. Launch the next wave.

## Status

- [x] Wave 1 Agent A — Export `EstimateSessionCost` in analyzer package
- [x] Wave 1 Agent B — Create `doctor` command
- [x] Wave 2 Agent C — Sessions inspect + summary stats + wire accurate cost

---

### Agent A — Completion Report

**What was implemented:**

1. **`internal/analyzer/outcomes.go`** — Renamed `estimateSessionCost` to `EstimateSessionCost` in two places: the function definition (line 164) and its comment, and the internal call site (line 85) inside `AnalyzeOutcomes`.

2. **`internal/analyzer/effectiveness.go`** — Updated the call site at line 255 inside `costPerCommit` from `estimateSessionCost` to `EstimateSessionCost`.

3. **`internal/analyzer/outcomes_test.go`** — Added `TestEstimateSessionCost_Exported` as a smoke test. It calls the exported function with 1,000,000 input tokens ($3.00 at $3/M) and 100,000 output tokens ($1.50 at $15/M) using `NoCacheRatio()`, and asserts the result is $4.50.

**Test results:**

```
=== RUN   TestEstimateSessionCost_Exported
--- PASS: TestEstimateSessionCost_Exported (0.00s)
PASS
ok  github.com/blackwell-systems/claudewatch/internal/analyzer  0.604s
```

All existing analyzer tests pass (`go test ./internal/analyzer/ -count=1`). Full build and vet pass with no errors.

**Deviations:** None. This was a pure rename with no logic changes.

**Out-of-scope dependencies:** None. No new packages or external dependencies were introduced.

---

### Agent B — Completion Report

**What was implemented:**

Created `internal/app/doctor.go` — a new standalone Cobra command (`claudewatch doctor`) that runs 8 health checks against the claudewatch setup and reports pass/fail for each.

**Checks implemented:**

1. **Claude home directory** — `os.Stat(cfg.ClaudeHome)` verifies the directory exists and is readable.
2. **Session data** — calls `claude.ParseAllSessionMeta(claudeHome)` and requires at least 1 result.
3. **Stats cache** — calls `claude.ParseStatsCache(claudeHome)` and requires a non-nil result.
4. **Scan paths** — iterates `cfg.ScanPaths`, calling `os.Stat` on each; generates one check per path. Falls through to a single failure check if no paths are configured.
5. **SQLite database** — `os.Stat(config.DBPath())` checks whether the database file exists.
6. **Watch daemon** — reads `config.ConfigDir()/watch.pid` with `os.ReadFile`, parses PID with `strconv.Atoi`, calls `os.FindProcess(pid)` then `proc.Signal(syscall.Signal(0))` to test process existence without signaling.
7. **CLAUDE.md coverage** — calls `scanner.DiscoverProjects(scanPaths)` and counts projects with/without `HasClaudeMD`. Passes if all projects have CLAUDE.md, or if coverage >= 50%. Reports the fraction and percentage.
8. **API key** — `os.Getenv("ANTHROPIC_API_KEY")` — passes if non-empty, masked in output for security.

**Output format:**

- Human-readable: `output.Section("Doctor")` header, then one line per check with a colored checkmark (✓) or X (✗), check name in bold, and detail in muted style. Summary line at end ("N/M checks passed") in green if all pass, yellow otherwise.
- JSON mode (`--json`): outputs a `doctorOutput` struct with `checks` array (name, passed, message), `passed` count, and `total` count.
- Both modes respect `--no-color` via `output.SetNoColor`.

**Command registration:** `rootCmd.AddCommand(doctorCmd)` in `init()`, consistent with all other commands.

**Verification results:**

```
go build ./...   # clean
go vet ./...     # clean
```

**Deviations from spec:**

- The spec mentioned "Import `syscall` only for this" and use `process.Signal(syscall.Signal(0))`. Implemented exactly as specified. `syscall.Signal(0)` is valid on both Unix and Windows in Go's syscall package, and the build verified cross-compilation is not required for this platform.
- The `min` helper function is defined locally in the file. Go 1.21+ has a built-in `min`, but defining a local function with the same name is valid Go (it shadows the builtin). `go vet` passes cleanly, confirming no issue.
- The CLAUDE.md coverage check passes when coverage is >= 50% (not just 100%), to avoid a noisy failure on large repos where some projects legitimately lack CLAUDE.md.

**Out-of-scope dependencies:** None. No new packages, no new files beyond `doctor.go`.

---

### Agent C — Completion Report

**What was implemented:**

**4a. Accurate cost estimation in `internal/app/sessions.go`:**

Removed the inline `estimatedCost()` method from `sessionRow`. Added an `EstimatedCost float64` field to `sessionRow` that is populated once at row construction time using `analyzer.EstimateSessionCost(s, pricing, cacheRatio)`. Stats-cache is loaded once at the top of `runSessions()` with `claude.ParseStatsCache(cfg.ClaudeHome)`; if loading fails or returns nil, `analyzer.NoCacheRatio()` is used. `analyzer.DefaultPricing["sonnet"]` is the default pricing tier. All cost references in sorting and rendering now read `r.EstimatedCost` directly instead of calling a method repeatedly. The `analyzer` package is now imported in `sessions.go`.

**4b. Summary stats footer in `renderSessions()`:**

After the table, a `Totals:` line is printed showing:
- Total cost across all displayed rows (`$X.XX`)
- Total commits
- Average friction per session (float, one decimal)
- Average duration in minutes

Format: `Totals: $X.XX cost · Y commits · Z.Z avg friction · Wm avg duration`

Rendered in `output.StyleBold` for emphasis. Added an additional hint line pointing users to the inspect feature.

**4c. `--inspect <session-id>` mode (`internal/app/sessions.go`):**

Changed `sessionsCmd.Args` from implicit `cobra.NoArgs` to `cobra.MaximumNArgs(1)`. The `Use` field was updated to `"sessions [session-id]"` and the `Long` description updated to mention inspect usage.

When a positional argument is provided, `runInspect()` is called. It scans all sessions for a full or prefix match (using `strings.HasPrefix`). Ambiguous prefix matches return an error. The `renderInspect()` function renders a structured detail view using `output.Section()` headers and `output.StyleLabel` / `output.StyleMuted` / `output.StyleBold` styled labels for:

- Session ID, project name, project path, date, duration
- User and assistant message counts
- Input/output tokens, estimated cost (4 decimal places for precision)
- Git commits, pushes, lines added/removed, files modified
- Tool counts (top 5 by usage, sorted descending then alphabetically)
- Friction types and counts (highlighted in `StyleWarning` if count > 2)
- Outcome (styled: green for "achieved", yellow for "not_achieved"), helpfulness, goal, summary
- First prompt (truncated to 200 chars with ellipsis)

Gracefully handles missing facet data (prints a muted "No facet data recorded" line). Respects `--json` flag: outputs the full `sessionRow` struct as JSON.

**4d. Accurate cost in `internal/watcher/watcher.go`:**

Replaced the inline `inputCost + outputCost` calculation in `Snapshot()` with `analyzer.EstimateSessionCost(s, pricing, cacheRatio)`. Stats-cache is loaded once per `Snapshot()` call (non-fatal: falls back to `NoCacheRatio()` on error or nil). `analyzer.DefaultPricing["sonnet"]` used as the default tier. The `analyzer` import was already present in `watcher.go`.

**Test results:**

```
go build ./...                 # clean
go vet ./...                   # clean
go test ./... -count=1         # all pass

?   	github.com/blackwell-systems/claudewatch/cmd/claudewatch	[no test files]
ok  	github.com/blackwell-systems/claudewatch/internal/analyzer	0.464s
?   	github.com/blackwell-systems/claudewatch/internal/app	[no test files]
ok  	github.com/blackwell-systems/claudewatch/internal/claude	0.685s
?   	github.com/blackwell-systems/claudewatch/internal/config	[no test files]
ok  	github.com/blackwell-systems/claudewatch/internal/fixer	0.236s
ok  	github.com/blackwell-systems/claudewatch/internal/output	0.649s
ok  	github.com/blackwell-systems/claudewatch/internal/scanner	0.870s
?   	github.com/blackwell-systems/claudewatch/internal/store	[no test files]
ok  	github.com/blackwell-systems/claudewatch/internal/suggest	0.517s
ok  	github.com/blackwell-systems/claudewatch/internal/watcher	1.590s
```

**Deviations from spec:**

- The `--inspect` mode is implemented as a positional argument (not a `--inspect` flag) to match the spec's stated syntax `claudewatch sessions <session-id>`. `cobra.MaximumNArgs(1)` enforces the arity.
- The `estimatedCost()` method was removed entirely rather than kept as a thin wrapper, since the cost is now a stored field. All call sites already use `r.EstimatedCost` directly.
- `renderInspect()` uses `output.Section()` for each sub-section header (Messages, Tokens & Cost, Git, Tools, Friction, Outcome & Satisfaction, First Prompt) to match the existing visual style of the codebase.

**Out-of-scope dependencies:** None. No new packages or files created. Only `internal/app/sessions.go` and `internal/watcher/watcher.go` were modified.
