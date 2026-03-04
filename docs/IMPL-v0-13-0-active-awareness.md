# IMPL: claudewatch v0.13.0 "Active Awareness"

<!-- scout v0.4.0 — generated 2026-03-04 -->

---

## Suitability Assessment

**Verdict: SUITABLE with caveats**

This is a SAW-appropriate project with three distinct feature areas that can be parallelized, but Feature #2 (contextual memory surfacing) requires investigation-first work to determine keyword extraction strategy.

### Gate Questions

1. **File decomposition** ✓
   - Feature #1 (expanded startup briefing) → `internal/app/startup.go` (Agent A)
   - Feature #2 (contextual memory surfacing) → New files in `internal/memory/` + integration in startup hook (Agent B, Wave 2)
   - Feature #3 (real-time dashboard) → `internal/app/hook.go` + new live stats aggregator (Agent C, Wave 2)

   Each agent owns disjoint file sets. Wave 1 scaffolds shared types if needed.

2. **Investigation-first items** ⚠
   - **Feature #2 requires investigation:** Keyword extraction strategy for matching user messages to task history is not yet defined. Options:
     - Simple substring matching on task identifiers
     - TF-IDF or keyword frequency analysis
     - Dependency: None (can implement simple substring matching first)
   - **Recommendation:** Start with simple substring/keyword matching (split on whitespace, match against `TaskIdentifier` field). More sophisticated NLP can be added later.
   - All other features are implementation-ready.

3. **Interface discoverability** ✓
   - Feature #1: Pure extension of existing `runStartup` function; no new interfaces
   - Feature #2: New function `SurfaceRelevantMemory(userMessage string, projectName string, memStore *WorkingMemoryStore) ([]TaskMemoryResult, error)` in `internal/memory/surface.go`
   - Feature #3: New function `ComputeSessionDashboard(activePath string, sessions []SessionMeta, facets []SessionFacet) (*DashboardStats, error)` in `internal/analyzer/dashboard.go`
   - All interfaces can be defined before implementation.

4. **Pre-implementation status check**
   - Feature #1: `internal/app/startup.go` exists and has:
     - ✓ Friction rate computation (lines 77-80)
     - ✓ Agent success rate computation (lines 93-108)
     - ✓ CLAUDE.md detection (lines 85-90)
     - ✗ **Missing:** Average tool errors per session display
     - ✗ **Missing:** Agent-specific failure warnings (e.g., "statusline-setup: 0% - DO NOT SPAWN")
     - ✗ **Missing:** New blocker count since last session
   - Feature #2: Cross-session memory infrastructure exists:
     - ✓ `get_task_history` MCP tool in `internal/mcp/memory_tools.go`
     - ✓ `WorkingMemoryStore` with `GetTaskHistory` method in `internal/store/working_memory.go`
     - ✗ **Missing:** Keyword extraction logic
     - ✗ **Missing:** Auto-injection into session context
   - Feature #3: PostToolUse hook exists in `internal/app/hook.go`:
     - ✓ Consecutive error detection (lines 75-82)
     - ✓ Context pressure detection (lines 85-91)
     - ✓ Cost velocity detection (lines 94-101)
     - ✓ Drift detection (lines 104-108)
     - ✗ **Missing:** Periodic dashboard display (currently only fires on threshold violations)
     - ✗ **Missing:** Session efficiency metrics (cost/commit ratio, commits so far)

   **Pre-implementation scan results:**
   - Total work items: 9 (3 existing + 6 missing)
   - Already implemented: 3 items (33%)
   - To-do: 6 items

5. **Parallelization value check** ✓
   - Feature #1 (startup.go): 20-30 min implementation
   - Feature #2 (memory surfacing): 40-50 min (investigation + implementation)
   - Feature #3 (real-time dashboard): 30-40 min
   - Sequential: ~90-120 min
   - SAW (2 waves, 3 parallel agents in Wave 2): ~60-75 min
   - **Savings:** 25-40 min, plus reduced merge conflicts

**Overall recommendation:** PROCEED with 2-wave SAW structure. Feature #2 uses simple keyword matching to avoid investigation delay.

**Verification test command:**
```bash
go build ./... && go vet ./... && go test ./...
```

---

## Scaffolds

No shared types need to be created in Wave 1. All agents work on independent code paths. However, we document shared types here for clarity:

### Existing types (no modifications needed):
- `claude.SessionMeta` (internal/claude/types.go:62-91)
- `claude.SessionFacet` (internal/claude/types.go:100-112)
- `claude.AgentTask` (internal/claude/types.go:145-156)
- `store.WorkingMemoryStore` (internal/store/working_memory.go)
- `memory.TaskMemory` (internal/memory/types.go)

### New types (created by agents as needed):

**DashboardStats** (Agent C creates in `internal/analyzer/dashboard.go`):
```go
package analyzer

type DashboardStats struct {
    CostSoFar       float64 // USD
    Commits         int
    CostPerCommit   float64 // USD
    ToolErrors      int
    AvgToolErrors   float64 // Avg at this point in session (from historical data)
    FrictionEvents  int
    TimeInDrift     float64 // minutes
    SessionDuration float64 // minutes
    Status          string  // "efficient", "adequate", "struggling"
    StatusEmoji     string  // "🟢", "🟡", "🔴"
}
```

**MemorySurfaceResult** (Agent B creates in `internal/memory/surface.go`):
```go
package memory

type MemorySurfaceResult struct {
    MatchedTasks []TaskMemory
    Keywords     []string // Keywords that triggered the match
}
```

---

## Known Issues

No blocking issues identified. All tests pass as of 2026-03-04:
```bash
$ go test ./...
ok      github.com/blackwell-systems/claudewatch/internal/analyzer      (cached)
ok      github.com/blackwell-systems/claudewatch/internal/app   0.637s
ok      github.com/blackwell-systems/claudewatch/internal/claude        1.149s
ok      github.com/blackwell-systems/claudewatch/internal/mcp   5.251s
ok      github.com/blackwell-systems/claudewatch/internal/memory        (cached)
```

**Potential friction points:**
- Feature #1: Regression warning logic (lines 110-127 of startup.go) may need adjustment if new metrics affect baseline comparisons. Mitigation: Keep existing regression logic unchanged, add new metrics separately.
- Feature #3: Hook cooldown (30s) may prevent dashboard from appearing frequently enough. Mitigation: Add separate "periodic display" flag that bypasses cooldown every N tool calls.

---

## Dependency Graph

```
Wave 1: No scaffolding needed (all agents are independent)

Wave 2: Three parallel agents

Agent A (startup-briefing)
  ├─ MODIFY internal/app/startup.go
  └─ READ (no writes):
      ├─ internal/claude/agents.go (ParseAgentTasks)
      ├─ internal/analyzer/commits.go (AnalyzeCommits)
      └─ internal/store/working_memory.go (for blocker count)

Agent B (memory-surfacing)
  ├─ CREATE internal/memory/surface.go
  ├─ CREATE internal/memory/surface_test.go
  ├─ MODIFY internal/app/startup.go (integrate memory surfacing)
  └─ READ (no writes):
      ├─ internal/store/working_memory.go (GetTaskHistory)
      └─ internal/memory/extract.go (TaskMemory type)

Agent C (dashboard)
  ├─ CREATE internal/analyzer/dashboard.go
  ├─ CREATE internal/analyzer/dashboard_test.go
  ├─ MODIFY internal/app/hook.go (add dashboard display)
  └─ READ (no writes):
      ├─ internal/claude/active_live.go (ParseLiveCommitAttempts, etc.)
      ├─ internal/analyzer/cost.go (EstimateSessionCost)
      └─ internal/claude/types.go (SessionMeta, SessionFacet)
```

**CRITICAL CONSTRAINT:** Agent A and Agent B both modify `internal/app/startup.go`. They MUST work on disjoint sections:
- **Agent A** owns lines 163-181 (line 1 identity + friction, line 2 CLAUDE.md/agent success, tools listing)
- **Agent B** adds NEW lines between line 181 and the final tool listing (line 179) to inject memory surfacing output

**Merge order:** Agent A commits first, Agent B rebases and inserts memory surfacing output after Agent A's changes.

---

## Interface Contracts

### Agent A: Expanded Startup Briefing

**Responsibility:** Extend `runStartup` in `internal/app/startup.go` to show expanded project health metrics.

**Files modified:**
- `internal/app/startup.go` (MODIFY lines 163-181)

**New output format (replace lines 174-180):**
```
╔ claudewatch | commitmux | 5 sessions | friction: HIGH (buggy_code dominant)
║ ⚠ HIGH FRICTION ENVIRONMENT - verify commands before execution
║ CLAUDE.md: ✓ | agent success: 96% | avg 112.6 tool errors/session (project baseline)
║
║ Agent failures by type:
║   • statusline-setup: 0% success (0/2 completed) - DO NOT SPAWN
║   • general-purpose: 100% success (42/42 completed) ✓
║
║ 3 new blockers since last session → call get_blockers() for details
║
║ tools: get_session_dashboard · get_project_health · get_task_history · get_blockers · extract_current_session_memory · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions
╚ PostToolUse hook active → fires on errors/context/cost → call get_session_dashboard
```

**Implementation details:**
1. Compute average tool errors per session:
   ```go
   var totalToolErrors int
   for _, sess := range projectSessions {
       totalToolErrors += sess.ToolErrors
   }
   avgToolErrors := float64(totalToolErrors) / float64(len(projectSessions))
   ```

2. Display agent failures with 0% success rate prominently:
   ```go
   // After existing agent success rate computation (lines 93-108)
   var failingAgents []string
   for agentType, summary := range byAgentType {
       if summary.SuccessRate == 0.0 {
           failingAgents = append(failingAgents, fmt.Sprintf("%s: 0%% success (%d/%d completed) - DO NOT SPAWN", agentType, 0, summary.Count))
       }
   }
   ```

3. Count new blockers since last session:
   ```go
   // Load working memory
   memoryPath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
   memStore := store.NewWorkingMemoryStore(memoryPath)
   wm, _ := memStore.Load()

   // Find most recent session end time
   var lastSessionEnd time.Time
   if len(projectSessions) > 1 {
       // sessions are sorted newest-first, so [1] is the second-most-recent
       lastSessionEnd = parseTime(projectSessions[1].StartTime).Add(time.Duration(projectSessions[1].DurationMinutes) * time.Minute)
   }

   newBlockerCount := 0
   for _, blocker := range wm.Blockers {
       if blocker.LastSeen.After(lastSessionEnd) {
           newBlockerCount++
       }
   }
   ```

4. Format friction warning:
   ```go
   if frictionRate >= 0.6 {
       fmt.Printf("║ ⚠ HIGH FRICTION ENVIRONMENT - verify commands before execution\n")
   } else if frictionRate >= 0.3 {
       fmt.Printf("║ ⚠ Moderate friction detected - watch for error patterns\n")
   }
   ```

**Contract outputs:**
- Modified stdout output with expanded metrics (no return value changes)
- No new exported functions

**Testing strategy:**
- Extend existing startup tests to verify new output lines
- Mock `WorkingMemoryStore` for blocker count test

---

### Agent B: Contextual Memory Surfacing

**Responsibility:** Auto-query task history when user message keywords match prior tasks, inject results into session start briefing.

**Files created:**
- `internal/memory/surface.go` (NEW)
- `internal/memory/surface_test.go` (NEW)

**Files modified:**
- `internal/app/startup.go` (ADD memory surfacing logic before tool listing)

**New function signature:**
```go
package memory

// SurfaceRelevantMemory extracts keywords from userMessage and queries task history.
// Returns matching tasks and the keywords that triggered matches.
// Uses simple substring matching: splits userMessage on whitespace, filters stop words,
// matches remaining tokens against TaskIdentifier fields case-insensitively.
func SurfaceRelevantMemory(userMessage string, projectName string, memStore *store.WorkingMemoryStore) (*MemorySurfaceResult, error)
```

**Implementation details:**

1. **Keyword extraction** (simple approach):
   ```go
   stopWords := map[string]bool{
       "the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
       "is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
       "to": true, "of": true, "in": true, "for": true, "on": true, "with": true,
       "as": true, "by": true, "at": true, "from": true, "this": true, "that": true,
       "it": true, "can": true, "will": true, "we": true, "i": true, "you": true,
   }

   words := strings.Fields(strings.ToLower(userMessage))
   var keywords []string
   for _, word := range words {
       cleaned := strings.Trim(word, ",.!?;:")
       if len(cleaned) >= 3 && !stopWords[cleaned] {
           keywords = append(keywords, cleaned)
       }
   }
   ```

2. **Query task history for each keyword:**
   ```go
   matchedMap := make(map[string]*TaskMemory)
   for _, keyword := range keywords {
       tasks, err := memStore.GetTaskHistory(keyword)
       if err != nil {
           continue
       }
       for i := range tasks {
           matchedMap[tasks[i].TaskIdentifier] = &tasks[i]
       }
   }

   var matched []TaskMemory
   for _, task := range matchedMap {
       matched = append(matched, *task)
   }
   ```

3. **Sort by LastUpdated descending:**
   ```go
   sort.Slice(matched, func(i, j int) bool {
       return matched[i].LastUpdated.After(matched[j].LastUpdated)
   })
   ```

**Integration into `startup.go`:**

Insert after line 181 (after regression warning, before tool listing):

```go
// Contextual memory surfacing: auto-query task history based on first user message.
// This only runs if there's an active session with a user message available.
activePath, _ := claude.FindActiveSessionPath(cfg.ClaudeHome)
if activePath != "" {
    activeSession, parseErr := claude.ParseActiveSession(activePath)
    if parseErr == nil && activeSession != nil {
        // Extract first user message from transcript
        transcript, _ := claude.ParseSingleTranscript(activePath)
        var firstUserMsg string
        for _, entry := range transcript {
            if entry.Type == "user" && entry.Message.Content != "" {
                firstUserMsg = entry.Message.Content
                break
            }
        }

        if firstUserMsg != "" {
            memoryPath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
            memStore := store.NewWorkingMemoryStore(memoryPath)
            surfaceResult, surfaceErr := memory.SurfaceRelevantMemory(firstUserMsg, projectName, memStore)

            if surfaceErr == nil && len(surfaceResult.MatchedTasks) > 0 {
                fmt.Printf("║\n")
                fmt.Printf("║ 📋 TASK HISTORY MATCH (keywords: %s)\n", strings.Join(surfaceResult.Keywords, ", "))
                fmt.Printf("║ Found %d prior attempt(s) on this project:\n", len(surfaceResult.MatchedTasks))

                for i, task := range surfaceResult.MatchedTasks[:min(3, len(surfaceResult.MatchedTasks))] {
                    statusIcon := "✓"
                    if task.Status == "abandoned" || task.Status == "in_progress" {
                        statusIcon = "✘"
                    }

                    timeAgo := formatTimeAgo(task.LastUpdated)
                    fmt.Printf("║   %s %s (%s, %s)\n", statusIcon, task.TaskIdentifier, task.Status, timeAgo)

                    if len(task.BlockersHit) > 0 {
                        fmt.Printf("║      Blockers: %s\n", strings.Join(task.BlockersHit, ", "))
                    }
                    if task.Solution != "" {
                        fmt.Printf("║      Solution: %s\n", truncate(task.Solution, 60))
                    }
                }

                fmt.Printf("║   → Call get_task_history(\"%s\") for full details\n", surfaceResult.Keywords[0])
            }
        }
    }
}
```

**Helper function:**
```go
func formatTimeAgo(t time.Time) string {
    d := time.Since(t)
    if d < 24*time.Hour {
        return fmt.Sprintf("%.0fh ago", d.Hours())
    } else if d < 7*24*time.Hour {
        return fmt.Sprintf("%.0fd ago", d.Hours()/24)
    } else {
        return fmt.Sprintf("%.0fw ago", d.Hours()/24/7)
    }
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen-3] + "..."
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

**Contract outputs:**
- `SurfaceRelevantMemory` function in `internal/memory/surface.go`
- Modified `runStartup` in `internal/app/startup.go` to inject memory matches

**Testing strategy:**
- Unit tests for keyword extraction (stop words, length filter)
- Unit tests for task matching logic
- Integration test with mock `WorkingMemoryStore`

---

### Agent C: Real-Time Friction Dashboard

**Responsibility:** Extend PostToolUse hook to display ongoing session metrics periodically (every 50 tool calls), not just on threshold violations.

**Files created:**
- `internal/analyzer/dashboard.go` (NEW)
- `internal/analyzer/dashboard_test.go` (NEW)

**Files modified:**
- `internal/app/hook.go` (ADD periodic dashboard display)

**New function signature:**
```go
package analyzer

// ComputeSessionDashboard aggregates live session metrics into a dashboard view.
// Uses activePath to read live tool calls, computes cost, commits, error rate,
// drift time, and assigns a status + emoji based on efficiency thresholds.
func ComputeSessionDashboard(activePath string, pricing claude.CostPricing) (*DashboardStats, error)
```

**Implementation details:**

1. **Read live session data:**
   ```go
   // Get commits (count tool calls with name "Bash" containing "git commit")
   commits := 0
   // Simplified: scan transcript for git commit tool calls
   transcript, _ := claude.ParseSingleTranscript(activePath)
   for _, entry := range transcript {
       if entry.Type == "tool_use" && entry.ToolName == "Bash" {
           // Check if command contains "git commit"
           if strings.Contains(entry.Input, "git commit") {
               commits++
           }
       }
   }

   // Get tool errors
   toolErrorStats, _ := claude.ParseLiveToolErrors(activePath)
   toolErrors := 0
   if toolErrorStats != nil {
       toolErrors = len(toolErrorStats.RecentErrors)
   }

   // Get cost (compute from token counts)
   meta, _ := claude.ParseActiveSession(activePath)
   costSoFar := 0.0
   if meta != nil {
       costSoFar = (float64(meta.InputTokens)/1e6)*pricing.InputPerMillion +
                   (float64(meta.OutputTokens)/1e6)*pricing.OutputPerMillion
   }

   // Get drift time
   driftStats, _ := claude.ParseLiveDriftSignal(activePath, 15)
   timeInDrift := 0.0
   if driftStats != nil && driftStats.Status == "drifting" {
       // Estimate: if 60% of last 15 calls were reads, assume 60% of recent time was drift
       // This is approximate; we don't have wall-clock time per tool call
       timeInDrift = 5.0 // placeholder; refine in implementation
   }

   // Get session duration (from first to last transcript entry)
   sessionDuration := 0.0
   if len(transcript) > 0 {
       first := parseTimestamp(transcript[0].Timestamp)
       last := parseTimestamp(transcript[len(transcript)-1].Timestamp)
       sessionDuration = last.Sub(first).Minutes()
   }
   ```

2. **Compute status:**
   ```go
   // Efficiency heuristic:
   // - Cost per commit < $1.00 = efficient (🟢)
   // - Cost per commit $1.00-$2.00 = adequate (🟡)
   // - Cost per commit > $2.00 OR tool errors > 20 OR drift > 30% = struggling (🔴)

   costPerCommit := 0.0
   if commits > 0 {
       costPerCommit = costSoFar / float64(commits)
   }

   driftPct := 0.0
   if sessionDuration > 0 {
       driftPct = (timeInDrift / sessionDuration) * 100
   }

   status := "adequate"
   emoji := "🟡"

   if costPerCommit < 1.00 && toolErrors < 10 && driftPct < 20 {
       status = "efficient"
       emoji = "🟢"
   } else if costPerCommit > 2.00 || toolErrors > 20 || driftPct > 30 {
       status = "struggling"
       emoji = "🔴"
   }
   ```

3. **Return dashboard struct:**
   ```go
   return &DashboardStats{
       CostSoFar:       costSoFar,
       Commits:         commits,
       CostPerCommit:   costPerCommit,
       ToolErrors:      toolErrors,
       FrictionEvents:  0, // TODO: count friction events from ParseLiveFriction
       TimeInDrift:     timeInDrift,
       SessionDuration: sessionDuration,
       Status:          status,
       StatusEmoji:     emoji,
   }, nil
   ```

**Integration into `hook.go`:**

Add periodic display logic at the start of `runHook`:

```go
func runHook(cmd *cobra.Command, args []string) {
    // Rate limiter: skip if within cooldown window.
    stampFile := os.ExpandEnv("$HOME/.cache/claudewatch-hook.ts")
    now := time.Now().Unix()
    if data, err := os.ReadFile(stampFile); err == nil {
        if last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
            if now-last < hookCooldownSeconds {
                return
            }
        }
    }
    _ = os.MkdirAll(os.ExpandEnv("$HOME/.cache"), 0o755)
    _ = os.WriteFile(stampFile, []byte(strconv.FormatInt(now, 10)), 0o644)

    cfg, err := config.Load(flagConfig)
    if err != nil {
        return
    }

    activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
    if err != nil || activePath == "" {
        return
    }

    // NEW: Periodic dashboard display (every 50 tool calls)
    toolCountFile := os.ExpandEnv("$HOME/.cache/claudewatch-toolcount")
    countData, _ := os.ReadFile(toolCountFile)
    lastCount := 0
    if len(countData) > 0 {
        lastCount, _ = strconv.Atoi(strings.TrimSpace(string(countData)))
    }

    // Get current tool count from transcript
    transcript, _ := claude.ParseSingleTranscript(activePath)
    currentCount := len(transcript)

    if currentCount > 0 && currentCount % 50 == 0 && currentCount != lastCount {
        // Display dashboard
        pricing := claude.CostPricing{
            InputPerMillion:  hookInputPerMillion,
            OutputPerMillion: hookOutputPerMillion,
        }
        dashboard, dashErr := analyzer.ComputeSessionDashboard(activePath, pricing)
        if dashErr == nil {
            fmt.Fprintf(os.Stderr, "\n╔═══════════════════════════════════════════════════╗\n")
            fmt.Fprintf(os.Stderr, "║  SESSION EFFICIENCY DASHBOARD                     ║\n")
            fmt.Fprintf(os.Stderr, "╠═══════════════════════════════════════════════════╣\n")
            fmt.Fprintf(os.Stderr, "║  Cost so far:       $%.2f\n", dashboard.CostSoFar)
            fmt.Fprintf(os.Stderr, "║  Commits:           %d\n", dashboard.Commits)
            if dashboard.Commits > 0 {
                targetSymbol := "✓"
                if dashboard.CostPerCommit > 1.50 {
                    targetSymbol = "⚠"
                }
                fmt.Fprintf(os.Stderr, "║  Cost per commit:   $%.2f (target: <$1.50) %s\n", dashboard.CostPerCommit, targetSymbol)
            }
            fmt.Fprintf(os.Stderr, "║  Tool errors:       %d\n", dashboard.ToolErrors)
            fmt.Fprintf(os.Stderr, "║  Time in session:   %.0f min\n", dashboard.SessionDuration)
            fmt.Fprintf(os.Stderr, "║\n")
            fmt.Fprintf(os.Stderr, "║  Status: %s %s\n", strings.ToUpper(dashboard.Status), dashboard.StatusEmoji)
            fmt.Fprintf(os.Stderr, "╚═══════════════════════════════════════════════════╝\n\n")
        }
        _ = os.WriteFile(toolCountFile, []byte(strconv.Itoa(currentCount)), 0o644)
    }

    // Existing threshold checks follow (consecutive errors, context pressure, etc.)
    // ...
}
```

**Contract outputs:**
- `ComputeSessionDashboard` function in `internal/analyzer/dashboard.go`
- Modified `runHook` in `internal/app/hook.go` to display dashboard periodically

**Testing strategy:**
- Unit tests for dashboard computation with mock transcript data
- Unit tests for status/emoji assignment logic
- Integration test verifying dashboard appears every 50 tool calls

---

## File Ownership Table

| File Path | Agent | Operation | Lines Modified | Notes |
|-----------|-------|-----------|----------------|-------|
| `internal/app/startup.go` | A | MODIFY | 163-181 | Expand metrics display |
| `internal/app/startup.go` | B | MODIFY | Insert after 181 | Add memory surfacing (merge after A) |
| `internal/memory/surface.go` | B | CREATE | - | New file for memory surfacing |
| `internal/memory/surface_test.go` | B | CREATE | - | Tests for memory surfacing |
| `internal/analyzer/dashboard.go` | C | CREATE | - | New file for dashboard computation |
| `internal/analyzer/dashboard_test.go` | C | CREATE | - | Tests for dashboard |
| `internal/app/hook.go` | C | MODIFY | 48-60 | Add periodic dashboard display |

**Conflict resolution strategy:**
- Agent A commits first (no dependencies)
- Agent B rebases on Agent A's commit before merging (B inserts new lines after A's changes)
- Agent C is fully independent (no overlapping files with A or B)

---

## Wave Structure

### Wave 1: (No scaffolding needed)
All agents are independent and work on disjoint file sets in Wave 2.

### Wave 2: Three parallel agents

**Agent A: startup-briefing**
- Expand project health metrics in `internal/app/startup.go`
- Show avg tool errors per session
- List failing agents with 0% success rate
- Count new blockers since last session
- Estimated time: 25-30 min

**Agent B: memory-surfacing**
- Create `internal/memory/surface.go` with keyword extraction
- Integrate memory surfacing into `internal/app/startup.go`
- Display matched task history in session start briefing
- Estimated time: 40-50 min

**Agent C: dashboard**
- Create `internal/analyzer/dashboard.go` with metrics aggregation
- Modify `internal/app/hook.go` to display dashboard every 50 tool calls
- Implement traffic light status logic (🟢/🟡/🔴)
- Estimated time: 30-40 min

**Total estimated time:** 95-120 min (Wave 2 agents run in parallel, so wall-clock time is 40-50 min for the longest agent)

---

## Agent Prompts

### Agent A: startup-briefing

```yaml
role: startup-briefing
goal: Expand project health metrics in SessionStart briefing
context: |
  You are extending the existing SessionStart hook in internal/app/startup.go
  to show expanded project health metrics. The hook already displays friction
  rate, agent success rate, and CLAUDE.md presence. You need to add:
  1. Average tool errors per session
  2. Agent-specific failure warnings (0% success rate agents)
  3. New blocker count since last session

  Current implementation: internal/app/startup.go lines 33-181
  Target users: Claude agents starting a new session (auto-injected context)

files:
  - internal/app/startup.go (MODIFY lines 163-181)

constraints:
  - Do NOT modify lines 1-162 (existing session filtering and computation logic)
  - Do NOT modify regression warning logic (lines 110-127)
  - Keep output format consistent with existing box-drawing characters (╔ ║ ╚)
  - Ensure backward compatibility (no breaking changes to hook signature)

verification:
  build: go build ./...
  test: go test ./internal/app -v -run TestRunStartup
  manual: |
    1. Run `claudewatch startup` in a project with historical sessions
    2. Verify expanded metrics appear in output
    3. Verify failing agents are listed with "DO NOT SPAWN" warning

dependencies: none
interfaces_consumed: none
interfaces_produced: none (pure output formatting changes)
```

---

### Agent B: memory-surfacing

```yaml
role: memory-surfacing
goal: Auto-query task history and inject relevant matches into session start briefing
context: |
  You are implementing contextual memory surfacing: when a user starts a session,
  extract keywords from their first message and automatically query task history
  for matching prior attempts. Inject results above the tool listing.

  The infrastructure exists:
  - WorkingMemoryStore in internal/store/working_memory.go
  - GetTaskHistory(query string) method on the store
  - TaskMemory type in internal/memory/types.go

  You need to:
  1. Create internal/memory/surface.go with SurfaceRelevantMemory function
  2. Implement simple keyword extraction (split on whitespace, filter stop words)
  3. Integrate into internal/app/startup.go (insert after line 181)

  This is Wave 2 Agent B and depends on Agent A completing first (merges after A).

files:
  - internal/memory/surface.go (CREATE)
  - internal/memory/surface_test.go (CREATE)
  - internal/app/startup.go (MODIFY - insert after line 181, before tool listing)

constraints:
  - Use simple substring matching (no external NLP libraries)
  - Filter stop words: "the", "a", "an", "and", "or", "is", "to", "of", etc.
  - Minimum keyword length: 3 characters
  - Show max 3 matched tasks in briefing output
  - Case-insensitive matching
  - Do NOT modify Agent A's changes (lines 163-181)

verification:
  build: go build ./...
  test: go test ./internal/memory -v -run TestSurfaceRelevantMemory
  manual: |
    1. Create a project with task history containing "authentication"
    2. Start a session with message "implement authentication"
    3. Verify task history match appears in startup briefing
    4. Verify keywords are extracted correctly

dependencies:
  - Agent A must complete first (startup.go lines 163-181)
  - Merge order: A commits, B rebases on A's commit

interfaces_consumed:
  - store.WorkingMemoryStore.GetTaskHistory(query string) ([]TaskMemory, error)
  - claude.FindActiveSessionPath(claudeHome string) (string, error)
  - claude.ParseActiveSession(path string) (*SessionMeta, error)

interfaces_produced:
  - memory.SurfaceRelevantMemory(userMessage string, projectName string, memStore *store.WorkingMemoryStore) (*MemorySurfaceResult, error)
```

---

### Agent C: dashboard

```yaml
role: dashboard
goal: Display real-time session efficiency dashboard in PostToolUse hook every 50 tool calls
context: |
  You are extending the PostToolUse hook in internal/app/hook.go to display
  a periodic session efficiency dashboard. The hook currently only fires on
  threshold violations (errors, context pressure, cost velocity, drift).

  You need to add periodic display logic that shows:
  - Cost so far (USD)
  - Commits completed
  - Cost per commit (with target threshold)
  - Tool errors
  - Session duration
  - Status: efficient (🟢) / adequate (🟡) / struggling (🔴)

  Display every 50 tool calls, bypassing the existing cooldown for threshold checks.

  Create internal/analyzer/dashboard.go with ComputeSessionDashboard function.

files:
  - internal/analyzer/dashboard.go (CREATE)
  - internal/analyzer/dashboard_test.go (CREATE)
  - internal/app/hook.go (MODIFY - add periodic display logic at start of runHook)

constraints:
  - Display every 50 tool calls (check tool count from transcript)
  - Do NOT interfere with existing threshold checks (consecutive errors, context pressure, etc.)
  - Use existing pricing constants from hook.go (hookInputPerMillion, hookOutputPerMillion)
  - Status thresholds:
    - Efficient: cost/commit < $1.00, errors < 10, drift < 20%
    - Struggling: cost/commit > $2.00 OR errors > 20 OR drift > 30%
    - Adequate: everything else
  - Store last-displayed tool count in ~/.cache/claudewatch-toolcount to avoid duplicate displays

verification:
  build: go build ./...
  test: go test ./internal/analyzer -v -run TestComputeSessionDashboard
  manual: |
    1. Start a Claude Code session with claudewatch PostToolUse hook enabled
    2. Execute 50+ tool calls
    3. Verify dashboard appears on stderr every 50 calls
    4. Verify status emoji matches efficiency criteria

dependencies: none (fully independent from Agents A and B)

interfaces_consumed:
  - claude.FindActiveSessionPath(claudeHome string) (string, error)
  - claude.ParseActiveSession(path string) (*SessionMeta, error)
  - claude.ParseLiveToolErrors(path string) (*LiveToolErrorStats, error)
  - claude.ParseLiveDriftSignal(path string, windowN int) (*LiveDriftStats, error)
  - claude.ParseSingleTranscript(path string) ([]TranscriptEntry, error)

interfaces_produced:
  - analyzer.ComputeSessionDashboard(activePath string, pricing claude.CostPricing) (*DashboardStats, error)
```

---

## Wave Execution Loop

**Orchestrator responsibilities:**
1. Verify all agents complete their work
2. Merge in order: Agent A → Agent B (rebase on A) → Agent C (independent)
3. Run full verification suite after each merge
4. Coordinate conflict resolution if needed

**Wave 2 execution:**

```bash
# Agent A: startup-briefing
cd /Users/dayna.blackwell/code/claudewatch
git checkout -b wave2-startup-briefing
# ... Agent A implements changes ...
go test ./internal/app -v -run TestRunStartup
git add internal/app/startup.go
git commit -m "feat(startup): expand project health metrics in session briefing

- Show avg tool errors per session
- List failing agents with 0% success rate
- Count new blockers since last session
- Format with HIGH FRICTION warning when rate >= 60%"
git push origin wave2-startup-briefing

# Agent B: memory-surfacing (wait for Agent A to push)
cd /Users/dayna.blackwell/code/claudewatch
git checkout main
git pull
git checkout -b wave2-memory-surfacing
git rebase wave2-startup-briefing  # Rebase on Agent A's branch
# ... Agent B implements changes ...
go test ./internal/memory -v -run TestSurfaceRelevantMemory
git add internal/memory/surface.go internal/memory/surface_test.go internal/app/startup.go
git commit -m "feat(memory): auto-surface relevant task history at session start

- Extract keywords from user message (stop word filtering)
- Query task history for matches
- Inject matched tasks into startup briefing
- Show max 3 matches with status, blockers, solutions"
git push origin wave2-memory-surfacing

# Agent C: dashboard (independent, can run in parallel with A and B)
cd /Users/dayna.blackwell/code/claudewatch
git checkout -b wave2-dashboard
# ... Agent C implements changes ...
go test ./internal/analyzer -v -run TestComputeSessionDashboard
git add internal/analyzer/dashboard.go internal/analyzer/dashboard_test.go internal/app/hook.go
git commit -m "feat(hook): display real-time efficiency dashboard every 50 tool calls

- Add ComputeSessionDashboard function in analyzer package
- Show cost, commits, cost/commit, tool errors, session duration
- Traffic light status: efficient (🟢) / adequate (🟡) / struggling (🔴)
- Display every 50 tool calls, separate from threshold alerts"
git push origin wave2-dashboard
```

**Merge order:**
1. Merge Agent A's branch to main first
2. Merge Agent B's branch (rebased on A) to main
3. Merge Agent C's branch to main
4. Run full test suite: `go test ./...`
5. Manual verification: start a claudewatch-enabled session and verify all three features work

---

## Orchestrator Post-Merge Checklist

After all Wave 2 agents complete:

- [ ] **Build verification:** `go build ./...` succeeds with no errors
- [ ] **Test suite:** `go test ./...` passes all tests
- [ ] **Lint check:** `go vet ./...` reports no issues
- [ ] **Agent A verification:** Run `claudewatch startup` in a project with history
  - [ ] Avg tool errors per session displayed
  - [ ] Failing agents listed with "DO NOT SPAWN" warning
  - [ ] New blocker count shown
- [ ] **Agent B verification:** Start a session with keywords matching task history
  - [ ] Memory surfacing output appears in startup briefing
  - [ ] Keywords extracted correctly (no stop words)
  - [ ] Max 3 matched tasks displayed
- [ ] **Agent C verification:** Enable PostToolUse hook, execute 50+ tool calls
  - [ ] Dashboard appears every 50 calls
  - [ ] Cost, commits, and status displayed correctly
  - [ ] Traffic light emoji matches efficiency criteria
- [ ] **Integration test:** All three features work together without conflicts
- [ ] **Regression check:** Existing features still work (get_project_health MCP tool, etc.)
- [ ] **Documentation update:** Update CHANGELOG.md with v0.13.0 features
- [ ] **Version bump:** Update version to v0.13.0 in appropriate files

---

## Status Tracking

| Agent | Status | Branch | Committed | Tests Pass | Manual Verification |
|-------|--------|--------|-----------|------------|---------------------|
| A (startup-briefing) | ⏳ Pending | wave2-startup-briefing | ⬜ | ⬜ | ⬜ |
| B (memory-surfacing) | ⏳ Pending | wave2-memory-surfacing | ⬜ | ⬜ | ⬜ |
| C (dashboard) | ⏳ Pending | wave2-dashboard | ⬜ | ⬜ | ⬜ |

**Legend:**
- ⏳ Pending
- 🔨 In Progress
- ✅ Complete
- ❌ Failed
- ⬜ Not Started

**Merge status:**
- [ ] Wave 2 Agent A merged to main
- [ ] Wave 2 Agent B merged to main (after A)
- [ ] Wave 2 Agent C merged to main
- [ ] All tests passing
- [ ] Manual verification complete
- [ ] v0.13.0 ready for release

---

## Notes for Orchestrator

**Critical merge dependency:** Agent B modifies `internal/app/startup.go` AFTER Agent A. Ensure Agent A's branch is merged first, then Agent B rebases on main before merging. Agent C is fully independent.

**Testing strategy:**
- Each agent runs its own unit tests before committing
- Orchestrator runs `go test ./...` after each merge
- Manual verification required for all three features (they're UI/output-focused)

**Potential blockers:**
- Agent B's memory surfacing requires an active session with a user message. If the startup hook runs before the first user message is written to the transcript, memory surfacing will be skipped. This is expected behavior (no-op if no user message exists yet).
- Agent C's dashboard display depends on tool count being divisible by 50. Test with a session that has >50 tool calls.

**Success metrics for v0.13.0:**
- Agent sees expanded health metrics in 100% of sessions (without calling get_project_health)
- Memory surfacing triggers in 40%+ of sessions with matching task history
- Dashboard visible every 50 tool calls in active sessions

**Post-release monitoring:**
- Track get_project_health MCP call frequency (should drop near zero)
- Track user feedback on memory surfacing relevance (false positives vs. useful matches)
- Monitor dashboard display frequency and user engagement
