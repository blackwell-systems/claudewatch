# IMPL: claudewatch hook — postToolCall Friction Hook

**Feature:** A `claudewatch hook` CLI subcommand designed for use as a Claude Code `postToolCall` shell hook. It tail-scans the active session's JSONL file for consecutive tool errors and exits non-zero with an actionable message when the threshold is exceeded.

**Design summary:** Two new units of code in two disjoint files: (1) `ParseLiveConsecutiveErrors` in `internal/claude/active_live.go` — a fast tail-scan function that returns the *current* consecutive error streak (not the historical max); (2) a `hookCmd` cobra subcommand in `internal/app/hook.go` — finds the active session path, calls `ParseLiveConsecutiveErrors`, and exits 0 (quiet) or 2 (threshold exceeded, prints one actionable line to stdout).

---

## Suitability Assessment

**Verdict: SUITABLE**

Two agents, fully disjoint file ownership, interface contract pre-defined. Agent A owns `internal/claude/active_live.go` (append one function) and Agent B owns `internal/app/hook.go` (create new file). Agent B calls the function Agent A implements, so they can run in a single wave — Agent B references the signature from this IMPL doc and will not have build errors because `ParseLiveConsecutiveErrors` can be stubbed or understood from the contract. No existing files require modification beyond the two files agents own.

**Estimated times:**
- Scout phase: ~20 min (this document)
- Agent execution: Wave 1, 2 agents in parallel, ~10–12 min each
- Merge & verification: ~5 min
- Total SAW time: ~35–40 min

**Note on single-wave parallelism:** Agent B depends on Agent A's symbol (`ParseLiveConsecutiveErrors`). In an isolated worktree, Agent B's worktree will not have the compiled symbol from Agent A. Agent B should write the `hook.go` file and tests treating the function as available (per the contract), note any build failure under `out_of_scope_build_blockers`, and mark verification as FAIL (build blocked on Agent A). The orchestrator resolves at merge. Alternatively, if the orchestrator wants a guarantee both agents pass verification independently, run Agent A first (Wave 1) and Agent B second (Wave 2) — both approaches are valid; the 2-agent parallel approach saves time at the cost of Agent B's verification state.

---

## Known Issues

None. `go test ./...` passes on `main` HEAD.

---

## Dependency Graph

```
[mod] internal/claude/active_live.go   ← appends ParseLiveConsecutiveErrors (uses existing readLiveJSONL)
         ↓
[new] internal/app/hook.go             ← new hookCmd cobra subcommand; calls ParseLiveConsecutiveErrors
                                          and claude.FindActiveSessionPath
```

Supporting dependencies (already exist, no modifications needed):
- `internal/claude/active.go` — `FindActiveSessionPath(claudeHome string) (string, error)` — owned by no agent, read-only
- `internal/config/config.go` — `config.Load(cfgFile string) (*Config, error)` — read-only
- `internal/app/root.go` — `rootCmd` var, `flagConfig` var — read-only (Agent B calls `rootCmd.AddCommand(hookCmd)` from its own `init()`)

---

## Interface Contracts

### Contract 1: `ParseLiveConsecutiveErrors` (owned by Agent A)

```go
// ParseLiveConsecutiveErrors tail-scans the last tailN entries of the JSONL
// file at path and returns the number of consecutive tool errors at the tail
// of the scan window (i.e., the current streak, not the historical maximum).
// Returns 0 if there are no errors or the file is empty.
// Uses readLiveJSONL internally; pass tailN <= 0 to use the default of 50.
func ParseLiveConsecutiveErrors(path string, tailN int) (int, error)
```

**Behavior:**
- Call `readLiveJSONL(path)` to get all entries, then take the last `tailN` entries (or all if fewer than `tailN`).
- Walk the tail forward. For each `user` entry: unmarshal as `UserMessage`, iterate `tool_result` blocks. Increment streak on `IsError == true`; reset streak to 0 on a non-error result.
- Return the streak value at the end of the walk (current trailing streak, not historical max).
- If `tailN <= 0`, use 50 as the default.
- If `readLiveJSONL` returns an error, return `(0, err)`.
- If the file is empty or has no complete lines, return `(0, nil)`.

**Key distinction from `ParseLiveToolErrors`:** `ParseLiveToolErrors.ConsecutiveErrs` returns the *historical maximum* streak across the entire session. `ParseLiveConsecutiveErrors` returns the *current trailing streak* at the tail of the scan window. This is what the hook needs: is the model currently stuck?

### Contract 2: `hookCmd` (owned by Agent B)

Cobra subcommand registered on `rootCmd`. Usage: `claudewatch hook`.

```
Flags: none (no --threshold flag; default threshold of 3 is hard-coded for now)
Exit codes:
  0 — no threshold exceeded (silent, no output)
  2 — consecutive error threshold exceeded; print one line to stdout
```

Output format on exit 2 (exact string, `\n`-terminated):
```
⚠ N consecutive tool errors — run get_live_friction for details
```
where N is the actual consecutive error count returned by `ParseLiveConsecutiveErrors`.

**Behavior:**
- Load config via `config.Load(flagConfig)` to get `cfg.ClaudeHome`.
- Call `claude.FindActiveSessionPath(cfg.ClaudeHome)`. If no active session found (returns `""`), exit 0 silently.
- Call `claude.ParseLiveConsecutiveErrors(activePath, 50)`. If error, exit 0 silently (hook must not disrupt workflow on failure).
- If `consecutiveErrs >= 3`, print the formatted line to stdout and exit with code 2 using `os.Exit(2)`.
- Otherwise exit 0 (cobra's default — no explicit `os.Exit(0)` needed).
- Do NOT return the error from `RunE`; errors must be swallowed silently (hook must never produce unexpected output that confuses Claude Code). Use `Run` (not `RunE`) or wrap with silent error handling.

---

## File Ownership Table

| File | Agent | Action | Notes |
|------|-------|--------|-------|
| `internal/claude/active_live.go` | A | modify (append) | Add `ParseLiveConsecutiveErrors` at bottom of file |
| `internal/claude/active_live_test.go` | A | modify (append) | Add tests for `ParseLiveConsecutiveErrors` |
| `internal/app/hook.go` | B | create | New cobra subcommand; `init()` registers on `rootCmd` |
| `internal/app/hook_test.go` | B | create | Tests for hookCmd registration and behavior |

No other files require modification.

---

## Wave Structure

**Single wave, 2 parallel agents.**

Wave 1:
- Agent A: Append `ParseLiveConsecutiveErrors` to `internal/claude/active_live.go` + tests
- Agent B: Create `internal/app/hook.go` + tests (references Agent A's contract; may fail build in isolated worktree — see note in Suitability section)

Merge: Orchestrator merges both worktree branches; runs `go build ./...`, `go vet ./...`, `go test ./... -race`.

---

## Agent Prompts

---

### Wave 1 Agent A: ParseLiveConsecutiveErrors

```
# Wave 1 Agent A: ParseLiveConsecutiveErrors

You are Wave 1 Agent A. Your task is to append one new exported function to
internal/claude/active_live.go and add corresponding tests.

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

If verification fails: Write error to completion report and exit immediately. Do NOT modify files.

## 1. File Ownership

**I1 — Disjoint File Ownership.** You own exactly these files:

- `internal/claude/active_live.go` — modify (append one function at the bottom)
- `internal/claude/active_live_test.go` — modify (append tests at the bottom)

Do not touch any other files.

## 2. Interfaces You Must Implement

```go
// ParseLiveConsecutiveErrors tail-scans the last tailN entries of the JSONL
// file at path and returns the number of consecutive tool errors at the tail
// of the scan window (the current trailing streak, not the historical maximum).
// Returns 0 if there are no errors or the file is empty.
// Uses readLiveJSONL internally. Pass tailN <= 0 to use the default of 50.
func ParseLiveConsecutiveErrors(path string, tailN int) (int, error)
```

## 3. Interfaces You May Call

```go
// readLiveJSONL is already defined in internal/claude/active_live.go (same file).
// It reads the full JSONL file, truncates at last newline, scans with a 10MB
// buffer, skips malformed lines, and returns []TranscriptEntry.
func readLiveJSONL(path string) ([]TranscriptEntry, error)

// UserMessage and AssistantMessage are defined in internal/claude/types.go.
// UserMessage.Content is []ContentBlock.
// ContentBlock.Type, ContentBlock.IsError, ContentBlock.ToolUseID are the
// relevant fields for tool_result blocks.
```

## 4. What to Implement

Read `internal/claude/active_live.go` first to understand the file structure,
existing types, and `readLiveJSONL`. Also read `internal/claude/types.go` for
`TranscriptEntry`, `UserMessage`, and `ContentBlock`.

Append `ParseLiveConsecutiveErrors` at the bottom of `active_live.go`.

**Algorithm:**
1. Call `readLiveJSONL(path)`. If error, return `(0, err)`. If nil/empty, return `(0, nil)`.
2. If `tailN <= 0`, set `tailN = 50`.
3. Take the last `tailN` entries: `tail := entries[max(0, len(entries)-tailN):]`
4. Walk `tail` forward. For each entry where `entry.Type == "user"` and `entry.Message != nil`:
   - Unmarshal as `UserMessage`.
   - Iterate `msg.Content`. For each block where `block.Type == "tool_result"`:
     - If `block.IsError == true`: increment `streak`.
     - Else: reset `streak = 0`.
5. Return `(streak, nil)`.

**Key distinction:** This returns the *trailing* streak (current state), not the historical max. `ParseLiveToolErrors` already returns the historical max in `ConsecutiveErrs`. This function answers "is the model currently stuck right now at the tail of the session?"

**Edge cases:**
- Empty file or no complete lines: return `(0, nil)`.
- tailN larger than entry count: use all entries.
- No `user` entries in tail: return `(0, nil)`.
- `readLiveJSONL` error: return `(0, err)`.

No new imports needed beyond what `active_live.go` already imports (`encoding/json` is already there).

## 5. Tests to Write

Append to `internal/claude/active_live_test.go`. Use the existing helpers
`writeLiveJSONL`, `mkAssistantToolUse`, `mkUserToolResult`.

1. `TestParseLiveConsecutiveErrors_NoErrors` — session with 3 successful tool results, expect 0
2. `TestParseLiveConsecutiveErrors_TrailingStreak` — 2 successes then 3 errors at tail, expect 3
3. `TestParseLiveConsecutiveErrors_StreakBrokenBySuccess` — 3 errors then 1 success then 2 errors at tail, expect 2 (trailing streak only)
4. `TestParseLiveConsecutiveErrors_TailNLimit` — session with 10 errors at start then 2 successes at tail, tailN=5; expect 0 (tail window contains only successes)
5. `TestParseLiveConsecutiveErrors_EmptyFile` — empty file, expect (0, nil)
6. `TestParseLiveConsecutiveErrors_DefaultTailN` — pass tailN=0, verify it uses 50 as default (use a session with 60 entries: first 15 are errors, last 45 are clean; expect 0 because the tail-50 window covers only clean entries)
7. `TestParseLiveConsecutiveErrors_AllErrors` — 5 error results, expect 5

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/claude/... -v -race
```

All must pass before reporting completion.

## 7. Constraints

- Do NOT import any new packages; the existing imports in `active_live.go` are sufficient.
- Do NOT modify any existing functions — only append new code.
- The function must be fast: it reads only the tail of the file (via `readLiveJSONL`'s full read then slice), which is acceptable because `readLiveJSONL` itself is the bottleneck and is already used by all other live parse functions.
- Return `(0, nil)` (not an error) when the active session file is empty or missing — the hook must not fail noisily.
- Do not use `os.Exit` anywhere in this file.

## 8. Report

Commit your changes before reporting:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
git add internal/claude/active_live.go internal/claude/active_live_test.go
git commit -m "wave1-agent-a: add ParseLiveConsecutiveErrors tail-scan function"
```

Append your completion report to `docs/IMPL-friction-hook.md` under
`### Agent A — Completion Report`:

```yaml
### Agent A — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-a
commit: {sha}
files_changed:
  - internal/claude/active_live.go
  - internal/claude/active_live_test.go
files_created: []
interface_deviations:
  - "[] or description of any deviation from the contract"
out_of_scope_deps: []
tests_added:
  - TestParseLiveConsecutiveErrors_NoErrors
  - TestParseLiveConsecutiveErrors_TrailingStreak
  - TestParseLiveConsecutiveErrors_StreakBrokenBySuccess
  - TestParseLiveConsecutiveErrors_TailNLimit
  - TestParseLiveConsecutiveErrors_EmptyFile
  - TestParseLiveConsecutiveErrors_DefaultTailN
  - TestParseLiveConsecutiveErrors_AllErrors
verification: PASS | FAIL ({command} — N/N tests)
```
```

---

### Wave 1 Agent B: hookCmd cobra subcommand

```
# Wave 1 Agent B: hookCmd cobra subcommand

You are Wave 1 Agent B. Your task is to create internal/app/hook.go (a new
cobra subcommand) and internal/app/hook_test.go.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
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

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

If verification fails: Write error to completion report and exit immediately. Do NOT modify files.

## 1. File Ownership

**I1 — Disjoint File Ownership.** You own exactly these files:

- `internal/app/hook.go` — create (new file)
- `internal/app/hook_test.go` — create (new file)

Do not touch any other files. The `init()` function in your new `hook.go` will
call `rootCmd.AddCommand(hookCmd)` — this writes to the `rootCmd` variable
defined in `internal/app/root.go`, which you do not own, but this is the
standard cobra registration pattern used by every other subcommand (tag.go,
scan.go, mcp.go, metrics.go) and does not require modifying `root.go`.

## 2. Interfaces You Must Implement

```go
// hookCmd is a cobra.Command registered on rootCmd.
// Usage: claudewatch hook
// Exit codes: 0 (quiet, no output) or 2 (threshold exceeded, one line to stdout)
var hookCmd *cobra.Command
```

## 3. Interfaces You May Call

```go
// Agent A delivers this — it is in package claude (internal/claude/active_live.go).
// Contract: tail-scans the last tailN entries, returns current trailing
// consecutive error streak. tailN=0 defaults to 50. Returns (0, nil) on
// empty file. Returns (0, err) on I/O error.
func claude.ParseLiveConsecutiveErrors(path string, tailN int) (int, error)

// Already exists in internal/claude/active.go.
// Returns ("", nil) if no active session found. Returns ("", err) only on
// unexpected I/O failure.
func claude.FindActiveSessionPath(claudeHome string) (string, error)

// Already exists in internal/config/config.go.
func config.Load(cfgFile string) (*Config, error)

// rootCmd and flagConfig are package-level vars in internal/app/root.go.
// All subcommands call rootCmd.AddCommand(...) from their own init().
var rootCmd *cobra.Command
var flagConfig string
```

## 4. What to Implement

Read these files before writing any code:
- `internal/app/tag.go` — canonical pattern for a simple subcommand with `init()` + `RunE`
- `internal/app/mcp.go` — minimal subcommand pattern
- `internal/app/root.go` — to understand `rootCmd`, `flagConfig`, and persistent flags

Create `internal/app/hook.go` in package `app`.

**hookCmd spec:**

```go
var hookCmd = &cobra.Command{
    Use:           "hook",
    Short:         "Check for consecutive tool errors (for use as a postToolCall shell hook)",
    Long:          `Tail-scans the active Claude Code session for consecutive tool errors.
Exit 0 if below threshold (silent). Exit 2 if threshold exceeded, with one
actionable line printed to stdout.

Intended for use as a Claude Code postToolCall shell hook:
  {"postToolCall": {"command": "claudewatch hook"}}`,
    SilenceUsage:  true,
    SilenceErrors: true,
    Run:           runHook,  // Use Run not RunE — errors must be swallowed silently
}

const hookThreshold = 3

func init() {
    rootCmd.AddCommand(hookCmd)
}

func runHook(cmd *cobra.Command, args []string) {
    cfg, err := config.Load(flagConfig)
    if err != nil {
        return // silent on config error
    }

    activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
    if err != nil || activePath == "" {
        return // no active session or error — silent
    }

    n, err := claude.ParseLiveConsecutiveErrors(activePath, 50)
    if err != nil || n < hookThreshold {
        return // below threshold or error — silent
    }

    fmt.Printf("⚠ %d consecutive tool errors — run get_live_friction for details\n", n)
    os.Exit(2)
}
```

**Exit code 2 is intentional.** Cobra's default exit code on `RunE` error is 1.
Since we use `Run` (not `RunE`) and call `os.Exit(2)` directly, cobra does not
interfere. Exit code 2 is the standard "threshold exceeded" signal for shell hooks.

**The hook must never produce unexpected output.** All errors are swallowed.
The only stdout output is the single threshold-exceeded line.

## 5. Tests to Write

Create `internal/app/hook_test.go` in package `app`.

**Note on testability:** `runHook` calls `os.Exit(2)`, which makes it hard to
test end-to-end in a subprocess-free way. The recommended pattern: test
registration and the command's static properties directly; for behavior tests,
use a subprocess (exec.Command) or test the logic indirectly via the threshold
constant.

1. `TestHookCmd_Registered` — verify `hookCmd` is registered on `rootCmd` by
   scanning `rootCmd.Commands()` for `Use == "hook"` (same pattern as
   `TestMCPCmd_Registered` in `mcp_test.go`)
2. `TestHookCmd_Use` — verify `hookCmd.Use == "hook"`
3. `TestHookCmd_SilenceErrors` — verify `hookCmd.SilenceErrors == true`
4. `TestHookThreshold_Value` — verify `hookThreshold == 3`

Note: end-to-end invocation tests (actually running the binary and checking
exit codes) are out of scope for unit tests and are handled by manual
verification. The unit tests above verify the command is correctly registered
and configured.

## 6. Verification Gate

**Before running:** This worktree does not contain Agent A's changes. The
`claude.ParseLiveConsecutiveErrors` symbol will not exist in the compiled
packages yet. This is expected parallel execution state.

If `go build ./...` fails because `ParseLiveConsecutiveErrors` is undefined:
1. Note it under `out_of_scope_build_blockers` in your report.
2. Your unit tests in `hook_test.go` do not call `ParseLiveConsecutiveErrors`
   directly, so `go test ./internal/app/... -run TestHookCmd` should still pass
   even if the full build fails — the test file only tests registration and
   static properties.

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claire/worktrees/wave1-agent-b
go vet ./internal/app/...
go test ./internal/app/... -v -race -run "TestHookCmd|TestHookThreshold"
```

If build fails on missing `ParseLiveConsecutiveErrors`, mark:
`verification: FAIL (build blocked on out-of-scope symbol — Agent A owns the fix)`

If the symbol happens to exist (e.g., worktrees were created after Agent A
committed), run the full suite:
```bash
go build ./...
go test ./... -v -race
```

## 7. Constraints

- Use `Run` not `RunE` so cobra does not intercept errors and print them.
- `os.Exit(2)` must be called directly — do not return an error and let cobra handle it.
- The command must never write to stderr.
- The only stdout output is the single threshold-exceeded line ending with `\n`.
- Do not add any CLI flags (no `--threshold`, no `--tail`). The defaults (threshold=3, tailN=50) are intentionally hard-coded for this initial implementation.
- `SilenceUsage: true` and `SilenceErrors: true` are required.
- The `hookThreshold` constant must be unexported and named exactly `hookThreshold` (tests reference it).
- Import `os` and `fmt` from stdlib; `github.com/blackwell-systems/claudewatch/internal/claude` and `github.com/blackwell-systems/claudewatch/internal/config` and `github.com/spf13/cobra`.

## 8. Report

Commit your changes before reporting:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
git add internal/app/hook.go internal/app/hook_test.go
git commit -m "wave1-agent-b: add claudewatch hook postToolCall subcommand"
```

Append your completion report to `docs/IMPL-friction-hook.md` under
`### Agent B — Completion Report`:

```yaml
### Agent B — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-b
commit: {sha}
files_changed: []
files_created:
  - internal/app/hook.go
  - internal/app/hook_test.go
interface_deviations:
  - "[] or description of any deviation from the contract"
out_of_scope_deps: []
out_of_scope_build_blockers:
  - "file: internal/claude/active_live.go, change: ParseLiveConsecutiveErrors not yet present, reason: Agent A owns this symbol; expected parallel state"
tests_added:
  - TestHookCmd_Registered
  - TestHookCmd_Use
  - TestHookCmd_SilenceErrors
  - TestHookThreshold_Value
verification: PASS | FAIL ({command} — N/N tests)
```
```

---

## Status Checklist

### Pre-Wave
- [x] Orchestrator creates worktrees: `wave1-agent-a`, `wave1-agent-b`
- [x] Orchestrator verifies `go test ./...` passes on `main` HEAD before launching agents

### Wave 1
- [x] Agent A: `ParseLiveConsecutiveErrors` implemented in `internal/claude/active_live.go`
- [x] Agent A: 7 tests appended to `internal/claude/active_live_test.go`
- [x] Agent A: Verification passes (`go test ./internal/claude/... -race`)
- [x] Agent A: Committed and reported
- [x] Agent B: `internal/app/hook.go` created with `hookCmd`
- [x] Agent B: `internal/app/hook_test.go` created with 4 registration tests
- [x] Agent B: Committed and reported

### Post-Merge
- [x] Orchestrator merges `wave1-agent-a` into `main` (or integration branch)
- [x] Orchestrator merges `wave1-agent-b` into same branch
- [x] `go build ./...` — clean
- [x] `go vet ./...` — clean
- [x] `go test ./... -race` — all pass
- [ ] Manual smoke test: `claudewatch hook` exits 0 in a clean session
- [ ] Manual smoke test: `claudewatch hook` exits 2 and prints the threshold line when 3+ consecutive errors exist in the active session
- [ ] Documentation note added (out of scope for agents): user adds `{"postToolCall": {"command": "claudewatch hook"}}` to `~/.claude/settings.json`

---

## Shell Hook Integration Note (Orchestrator Reference Only)

The following is guidance for the human operator. No agent implements this.

**settings.json hook entry:**
```json
{
  "hooks": {
    "postToolCall": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "claudewatch hook"
          }
        ]
      }
    ]
  }
}
```

**Rate limiting (optional):** Because `postToolCall` fires on every tool use,
a rate limiter prevents the binary from being invoked too frequently. Example
wrapper script (saves ~30ms per tool use by skipping when cooldown is active):

```bash
#!/bin/bash
# ~/.local/bin/claudewatch-hook
STAMP="$HOME/.cache/claudewatch-hook.ts"
NOW=$(date +%s)
if [ -f "$STAMP" ]; then
  LAST=$(cat "$STAMP")
  if [ $((NOW - LAST)) -lt 30 ]; then
    exit 0
  fi
fi
echo "$NOW" > "$STAMP"
claudewatch hook
```

The binary itself is fast (tail-scan only) but the rate limiter reduces
invocations to at most one per 30 seconds.

---

## Completion Reports

<!-- Agent completion reports are appended here by agents -->

### Agent A — Completion Report

status: complete
worktree: .claude/worktrees/wave1-agent-a
commit: 9349e47
files_changed:
  - internal/claude/active_live.go
  - internal/claude/active_live_test.go
files_created: []
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestParseLiveConsecutiveErrors_NoErrors
  - TestParseLiveConsecutiveErrors_TrailingStreak
  - TestParseLiveConsecutiveErrors_StreakBrokenBySuccess
  - TestParseLiveConsecutiveErrors_TailNLimit
  - TestParseLiveConsecutiveErrors_EmptyFile
  - TestParseLiveConsecutiveErrors_DefaultTailN
  - TestParseLiveConsecutiveErrors_AllErrors
verification: PASS (go test ./internal/claude/... -v -race — 7/7 new tests)

### Agent B — Completion Report

status: complete
worktree: .claude/worktrees/wave1-agent-b
commit: 8a53cc1
files_changed: []
files_created:
  - internal/app/hook.go
  - internal/app/hook_test.go
interface_deviations: []
out_of_scope_deps: []
out_of_scope_build_blockers:
  - "internal/claude/active_live.go: ParseLiveConsecutiveErrors not yet present — Agent A owns this symbol; expected parallel state"
tests_added:
  - TestHookCmd_Registered
  - TestHookCmd_Use
  - TestHookCmd_SilenceErrors
  - TestHookThreshold_Value
verification: PARTIAL (go test ./internal/app/... -run TestHookCmd|TestHookThreshold — build blocked on Agent A's ParseLiveConsecutiveErrors symbol; app package tests cannot compile until Agent A merges)
