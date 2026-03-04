# claudewatch Behavioral Enhancement Roadmap

> **Note:** This roadmap focuses on **behavioral interventions** (auto-injection, guardrails, forced reflection). For **analytical capabilities** (predictive intelligence, cost forecasting, platform features), see [docs/ROADMAP.md](docs/ROADMAP.md).

**Vision:** Transform claudewatch from a passive toolkit into an active cognitive scaffold that shapes AI agent behavior by making good behavior the default path.

**Current state:** Tools exist but require opt-in (agents must remember to call `get_project_health`, `get_blockers`, etc.). Adoption is low even with explicit instructions. SessionStart hook shows basic project info but not detailed health metrics.

**Target state:** Data is unavoidable, bad behavior is blocked, reflection is forced, learning is automatic.

---

## Core Principle

**Move from OPTIONAL to UNAVOIDABLE:**

- ❌ Tools agents can call → ✅ Data agents can't avoid seeing
- ❌ Suggestions agents can ignore → ✅ Blocks agents must acknowledge
- ❌ Reactive querying → ✅ Proactive injection
- ❌ "You should..." → ✅ "Here's what's happening..."
- ❌ Memory on demand → ✅ Memory when relevant
- ❌ End-of-session insights → ✅ Real-time feedback

---

## Phase 1: Foundation - Make Data Visible (v0.13.0)

**Goal:** Eliminate the need to query - inject baseline awareness by default

### 1.1 Auto-inject project health into session start briefing
**Impact:** 🔥🔥🔥 High - Fixes the core adoption problem
**Effort:** 🛠 Low - Hook already exists in `internal/app/startup.go`, extend it
**Dependencies:** None

**Current state:** SessionStart hook shows friction label (HIGH/moderate/low), top friction type, CLAUDE.md presence, agent success rate, and tips. Missing: detailed metrics, agent-specific failures, blocker counts.

**Implementation:**
- Extend `internal/app/startup.go` to show expanded metrics
- Change from "tip: call get_project_health" to showing the data directly
- Format:
  ```
  ╔ claudewatch | commitmux | friction: 40% ↑ (was 32%)
  ║ ⚠ HIGH FRICTION ENVIRONMENT - verify commands before execution
  ║ Top errors: buggy_code (45%), retry:Bash (32%)
  ║ Avg 112 tool errors/session - you are error-prone here
  ║
  ║ Agent failures: statusline-setup (0% success) - DO NOT SPAWN
  ║ 3 new blockers since last session → call get_blockers()
  ║ Last checkpoint: 45 min ago in session abc123
  ```

**Success metric:** Agent MCP calls to `get_project_health` drop to near zero (because data is already provided)

**Priority:** ⭐⭐⭐ DO FIRST

### 1.2 Contextual memory surfacing
**Impact:** 🔥🔥🔥 High - Reduces repeated failures
**Effort:** 🛠🛠 Medium - Pattern matching + injection
**Dependencies:** Task history extraction (exists)

**Implementation:**
- Hook into user message parsing at session start
- Extract keywords/topics from user message (first prompt or resume context)
- Query `get_task_history` automatically when keywords match prior tasks
- Inject matches into context above user message:
  ```
  📋 TASK HISTORY MATCH: "authentication"

  Found 2 prior attempts on this project:

    ✘ Session abc123 (1w ago): Tried JWT, hit rate limits
       Blocker: Auth0 quota exceeded, switched to sessions
       Status: Abandoned

    ✓ Session def456 (3d ago): Implemented session-based auth
       Solution: express-session + Redis
       Status: Completed, 4 commits

    Recommendation: Use sessions approach (proven success)
  ```

**Success metric:** "We tried this before" user complaints drop by 50%

**Priority:** ⭐⭐ DO NEXT

### 1.3 Real-time friction dashboard in briefing
**Impact:** 🔥🔥 Medium - Increases awareness during session
**Effort:** 🛠 Low - Use existing session stats
**Dependencies:** Live session reading (exists)

**Implementation:**
- Add live metrics to PostToolUse hook output (every 50 tool calls or on ⚠ trigger)
- Show:
  ```
  SESSION EFFICIENCY DASHBOARD
  ─────────────────────────────
  Cost so far:       $2.40
  Commits:           2
  Cost per commit:   $1.20 (target: <$1.50) ✓
  Tool errors:       8 (avg: 4 at this point) ⚠
  Friction events:   3 (error loops, retries)
  Time in drift:     12 min (20% of session) ⚠

  Status: ADEQUATE - watch error rate
  ```
- Traffic light status: 🟢 efficient / 🟡 adequate / 🔴 struggling

**Success metric:** Agents reference efficiency metrics in decision-making

**Priority:** DO FIRST

**Deliverable:** v0.13.0 - "Active Awareness Release"

---

## Phase 2: Guardrails - Prevent Bad Behavior (v0.14.0)

**Goal:** Block known failures before they happen

### 2.1 Agent spawn blocking
**Impact:** 🔥🔥🔥 High - Prevents wasted cost on failing agents
**Effort:** 🛠🛠 Medium - Requires PreToolUse hook implementation
**Dependencies:** Agent performance data (exists)

**Implementation:**
- Add `PreToolUse` hook that intercepts tool calls before execution
- On `Agent` tool with `subagent_type` parameter:
  - Query agent success rate for that type on current project
  - If <30% success rate, return error blocking the spawn
  - Inject failure context:
    ```
    ✘ BLOCKED: statusline-setup agents fail 100% on this project (0/2 success)
      Last failures: session abc123 (3h ago), session def456 (1d ago)
      Common error: "permission denied writing ~/.config/nvim/lua/..."

      Alternatives:
      - Manual configuration (documented in CLAUDE.md)
      - Ask user to run setup script directly
    ```
- Provide override mechanism for user confirmation if needed

**Success metric:** Zero spawns of agents with <30% success rate without user override

**Priority:** ⭐ DO NEXT

### 2.2 Drift intervention
**Impact:** 🔥🔥 Medium-High - Reduces wasted time exploring without implementing
**Effort:** 🛠 Low - PostToolUse hook extension
**Dependencies:** Drift detection (exists via `get_drift_signal`)

**Implementation:**
- PostToolUse hook tracks read/write ratio in rolling 15-tool window
- After 10 consecutive reads (no writes), inject warning:
  ```
  🛑 DRIFT DETECTED: 10 reads, 0 writes in 15 minutes

  You are exploring without implementing. Common causes:
  - Unclear requirements → Ask user for clarification
  - Overwhelmed by complexity → Scope down to smallest first step
  - Avoiding implementation → What's blocking you?

  Actions:
  [ ] Ask user for direction
  [ ] Scope down and implement smallest piece
  [ ] Call get_blockers() - is there a known issue?
  ```
- Escalate every 5 additional reads without writes

**Success metric:** Drift sessions (>20% time in read-only mode) drop by 40%

**Priority:** DO FIRST

### 2.3 Repetitive error blocking
**Impact:** 🔥🔥 Medium - Stops error loops early
**Effort:** 🛠 Low - PostToolUse hook logic
**Dependencies:** Live friction tracking (exists)

**Implementation:**
- Track (tool_name, error_category) tuples in current session
- After 3rd occurrence of same pattern:
  ```
  ⚠ PATTERN DETECTED: 3 Bash retries in 10 min

    Your friction rate this session: 40%
    Your normal rate: 25%
    → You're struggling 60% more than usual

    Common cause on this project: commands not verified before execution

    Action: Call get_blockers() to check for known issues
  ```

**Success metric:** Sessions with 5+ repetitive errors drop by 50%

**Priority:** DO FIRST

**Deliverable:** v0.14.0 - "Guardrails Release"

---

## Phase 3: Metacognition - Force Reflection (v0.15.0)

**Goal:** Build memory habits through mandatory pauses

### 3.1 Reflection checkpoints
**Impact:** 🔥🔥🔥 High - Dramatically improves memory extraction quality and frequency
**Effort:** 🛠🛠 Medium - Requires interruption mechanism
**Dependencies:** `extract_current_session_memory` (exists)

**Implementation:**
- Timer-based triggers: 30 min, 60 min, 90 min
- PostToolUse hook injects checkpoint prompt:
  ```
  ⏸ REFLECTION CHECKPOINT (30 min elapsed)

  What have you learned that's worth preserving?
  - New blockers discovered and solutions?
  - Patterns that worked or failed?
  - Architectural insights about this codebase?

  Template:
  ## Blocker: [brief name]
  File: [path if relevant]
  Issue: [what went wrong]
  Solution: [how it was fixed]

  ## Pattern: [what worked/failed]
  Context: [when this applies]
  Outcome: [result]
  ```
- Agent writes response, system auto-calls `extract_current_session_memory` with notes
- Snooze option: "Remind me in 15 min"

**Success metric:** Memory extraction rate goes from <10% to 60% of sessions >30min

**Priority:** ⭐ DO NEXT

### 3.2 Success pattern learning
**Impact:** 🔥🔥 Medium - Reinforces effective behaviors
**Effort:** 🛠🛠🛠 High - Requires pattern analysis engine
**Dependencies:** Historical session data (exists)

**Implementation:**
- New analyzer: `internal/analyzer/patterns.go`
- Correlate tool usage sequences with outcomes (commits, friction, cost)
- Store "pattern effectiveness" by project in local DB
- Inject when relevant pattern detected in current session:
  ```
  💡 YOUR PATTERN: When you use Edit tool + tests on this project
     Success rate: 92% (23/25 sessions)
     Avg cost: $1.20

     When you use Write tool (rewrites) on this project
     Success rate: 60% (6/10 sessions)
     Avg cost: $2.80

     → Prefer Edit tool for this codebase
  ```
- Track: Edit→Test, Read→Write, Grep→Read→Edit sequences

**Success metric:** Adoption rate of high-success patterns (>80% success) increases 30%

**Priority:** BACKLOG

### 3.3 Cost/benefit visibility
**Impact:** 🔥 Low-Medium - Awareness, not behavior change
**Effort:** 🛠 Low - Dashboard formatting (reuse from 1.3)
**Dependencies:** Live session stats (exists)

**Implementation:**
- Extend 1.3 dashboard with comparison to historical averages
- Add target thresholds (cost/commit <$1.50, friction <30%, etc.)
- Show trend arrows (↑↓) vs recent sessions

**Success metric:** Agents reference cost metrics when making tool choices

**Priority:** BACKLOG

**Deliverable:** v0.15.0 - "Metacognition Release"

---

## Phase 4: Collective Intelligence (v0.16.0)

**Goal:** Learn from aggregate patterns across users (opt-in)

### 4.1 Anonymized blocker sharing
**Impact:** 🔥🔥🔥 High - Crowdsourced solutions to common problems
**Effort:** 🛠🛠🛠🛠 Very High - Requires opt-in telemetry infrastructure
**Dependencies:** Privacy-preserving aggregation system

**Implementation:**
- New command: `claudewatch share --anonymous`
- Privacy safeguards:
  - Hash project names (preserve uniqueness, not identity)
  - Strip absolute file paths (keep relative structure only)
  - Remove all user identifiers, session IDs, timestamps
  - Only upload: blocker pattern, solution, success/failure
- Upload to central registry (hosted service or distributed DHT)
- Query registry in `get_blockers()`:
  ```
  🌐 COLLECTIVE INSIGHT: This blocker seen 47 times across 12 projects

  Most effective solution (80% success):
    "Run go fmt before git commit, add pre-commit hook"

  Attempted solutions that failed:
    - Manual formatting (40% success - forgot steps)
    - IDE auto-format (55% success - inconsistent)

  Average time to resolve: 8 minutes
  ```
- Local-first fallback: if no network, use local data only

**Success metric:**
- 1000+ shared blockers in registry
- Blocker resolution time drops 40% for participants

**Priority:** LONG TERM

### 4.2 Pattern effectiveness benchmarking
**Impact:** 🔥🔥 Medium - Community learning from aggregate patterns
**Effort:** 🛠🛠🛠 High - Aggregation + privacy + analysis
**Dependencies:** Anonymized sharing infrastructure (4.1)

**Implementation:**
- Aggregate: (tool sequence, project type, language) → outcomes
- Community benchmarks:
  ```
  📊 COMMUNITY BENCHMARK: Go projects

  Users who use "Edit → Test → Commit" pattern:
    Success rate: 87%
    Avg cost/commit: $1.15

  Users who use "Write → Manual verify → Commit" pattern:
    Success rate: 72%
    Avg cost/commit: $1.85

  Your pattern: Edit → Test (matches high-performing group)
  ```
- Show percentile rankings for friction rate, cost efficiency

**Success metric:** Participants adopt community best practices, friction drops 25%

**Priority:** LONG TERM

**Deliverable:** v0.16.0 - "Collective Intelligence Release"

---

## Phase 5: Adaptive Behavior (v0.17.0)

**Goal:** Personalized nudges based on individual agent patterns

### 5.1 Behavioral nudges
**Impact:** 🔥🔥 Medium - Reduces friction through personalization
**Effort:** 🛠🛠 Medium - Personalization engine + feedback loop
**Dependencies:** Individual behavior tracking across sessions

**Implementation:**
- Track effectiveness of each nudge type per agent/project:
  - Was the nudge shown? (yes)
  - Was the action taken? (yes/no)
  - Did it improve outcome? (friction/cost comparison)
- Learn: which nudges work for this agent on this project
- Adapt: show more of what works, suppress what's ignored
- Personalized friction reduction strategies
- Example: if agent always ignores drift warnings but responds to blocker alerts, prioritize blocker notifications

**Success metric:** Nudge response rate increases from 40% to 70%

**Priority:** LONG TERM

### 5.2 Proactive task history surfacing
**Impact:** 🔥🔥🔥 High - Zero-friction memory access
**Effort:** 🛠🛠🛠 High - Intent detection + ML
**Dependencies:** Contextual memory surfacing (1.2)

**Implementation:**
- ML model (or heuristic classifier): user message → task intent
- Categories: authentication, API integration, database migration, testing, etc.
- Auto-query task history on high-confidence intent match
- Inject BEFORE agent processes message (not after agent asks)
- Feedback loop: did agent use the information? (track if referenced in response)
- Learn from feedback to improve classification

**Success metric:**
- Task history surfaced automatically in 80% of relevant cases
- Agent references prior attempts in 70% of those cases

**Priority:** RESEARCH / LONG TERM

**Deliverable:** v0.17.0 - "Adaptive Behavior Release"

---

## Implementation Priority Matrix

### High Impact, Low Effort (DO FIRST - v0.13.0)
- ⭐⭐⭐ 1.1 Auto-inject project health
- 1.3 Real-time friction dashboard
- 2.2 Drift intervention
- 2.3 Repetitive error blocking

### High Impact, Medium Effort (DO NEXT - v0.14.0-v0.15.0)
- ⭐⭐ 1.2 Contextual memory surfacing
- ⭐ 2.1 Agent spawn blocking
- ⭐ 3.1 Reflection checkpoints

### Medium Impact (BACKLOG - v0.15.0-v0.16.0)
- 3.2 Success pattern learning
- 3.3 Cost/benefit visibility
- 5.1 Behavioral nudges

### High Effort, Long Term (RESEARCH - v0.16.0+)
- 4.1 Anonymized blocker sharing
- 4.2 Pattern effectiveness benchmarking
- 5.2 Proactive task history surfacing

---

## Milestones & Timeline

### v0.13.0 - "Active Awareness" (Target: 2-3 weeks)
**Deliverables:**
- Auto-inject project health into session start briefing
- Contextual memory surfacing (keyword-based)
- Real-time friction dashboard in PostToolUse hook

**Success metrics:**
- Agent sees baseline data in 100% of sessions (without calling get_project_health)
- Memory surfacing triggers in 40% of sessions with prior task history
- Friction dashboard visible every 50 tool calls

**Technical approach:**
- Extend `internal/hooks/session_start.go` to call MCP tools internally
- Add keyword extraction to user message parser
- Extend PostToolUse hook with dashboard formatting

---

### v0.14.0 - "Guardrails" (Target: 3-4 weeks after v0.13.0)
**Deliverables:**
- Agent spawn blocking for low-success agent types
- Drift intervention alerts
- Repetitive error blocking

**Success metrics:**
- 50% reduction in spawns of agents with <30% success rate
- 40% reduction in drift sessions (>20% time in read-only)
- 50% reduction in sessions with 5+ repetitive errors

**Technical approach:**
- Implement PreToolUse hook for tool interception
- Add rolling window tracking to PostToolUse hook
- Pattern detection for (tool, error) tuples

---

### v0.15.0 - "Metacognition" (Target: 4-6 weeks after v0.14.0)
**Deliverables:**
- Reflection checkpoints (timer-based)
- Success pattern learning engine
- Enhanced cost/benefit visibility

**Success metrics:**
- 60% of sessions >30min have memory extraction
- 30% increase in adoption of high-success patterns (>80% success rate)
- Cost/commit improves 15% from baseline

**Technical approach:**
- Timer integration with PostToolUse hook
- New pattern analyzer: `internal/analyzer/patterns.go`
- Historical pattern correlation engine

---

### v0.16.0 - "Collective Intelligence" (Target: 8-12 weeks after v0.15.0)
**Deliverables:**
- Anonymized blocker sharing (opt-in)
- Community pattern benchmarks
- Privacy-preserving aggregation

**Success metrics:**
- 1000+ shared blockers in registry
- 100+ active participants
- 40% faster blocker resolution for participants
- 25% friction reduction through community best practices

**Technical approach:**
- Central registry design (hosted service or DHT)
- Privacy safeguards: hashing, path stripping, anonymization
- Local-first fallback for offline operation
- Opt-in consent flow

---

### v0.17.0 - "Adaptive Behavior" (Target: 12+ weeks after v0.16.0)
**Deliverables:**
- Personalized behavioral nudges
- Proactive task history surfacing
- Feedback-driven optimization

**Success metrics:**
- Nudge response rate 70% (up from 40%)
- Task history surfaced automatically in 80% of relevant cases
- 50% friction rate reduction vs baseline

**Technical approach:**
- Personalization engine: track nudge effectiveness per agent
- Intent classification (ML or heuristic-based)
- Feedback loop: measure if surfaced info was used

---

## Next Actions

### Start Today
1. Create GitHub issues for Phase 1 features (1.1, 1.2, 1.3)
2. Draft technical spec for auto-injected project health format
3. Design contextual memory surfacing keyword matching algorithm

### This Week
1. Implement 1.1 (auto-inject project health)
   - Extend `internal/hooks/session_start.go`
   - Call `mcp.GetProjectHealth()` internally
   - Format rich baseline output
2. Test with dogfooding: verify agent sees data without querying
3. Create branch: `feature/auto-inject-health`

### This Month
1. Complete Phase 1 (v0.13.0-alpha)
2. Dogfood for 1 week, gather feedback
3. Ship v0.13.0 stable
4. Begin Phase 2: prototype agent spawn blocking

### This Quarter
1. Ship v0.13.0 and v0.14.0
2. Begin Phase 3 (reflection checkpoints)
3. Gather community feedback on injection vs query tradeoffs
4. Refine behavioral nudge strategies based on real usage

---

## The North Star

### By v0.15.0, an agent using claudewatch should:
- ✅ **SEE** project health without asking (auto-injected at session start)
- ✅ **KNOW** past attempts when relevant (auto-surfaced on keyword match)
- ✅ **STOP** before repeating failures (blocked by guardrails)
- ✅ **PAUSE** to reflect regularly (forced checkpoints every 30min)
- ✅ **LEARN** from patterns (shown personal success rates)

### By v0.17.0, add:
- ✅ Learn from the community (collective intelligence, opt-in)
- ✅ Adapt to individual behavior (personalized nudges)
- ✅ Zero-friction memory (proactive surfacing)

---

## Open Questions

1. **PreToolUse hook:** Does Claude Code support this? If not, can we intercept via MCP server?
2. **Interruption mechanism:** How do we force a pause for reflection checkpoints? PostToolUse hook output only?
3. **Central registry:** Self-hosted vs cloud service? DHT for decentralization?
4. **Intent classification:** ML model (cost/complexity) vs heuristic rules (simpler)?
5. **Feedback loop:** How do we measure if agent used surfaced information? Parse assistant message?

---

## Contributing

Interested in implementing a roadmap feature? See:
- [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines
- [Phase 1 tracking issue](#) for current work
- [Roadmap discussions](#) for open questions and design debates

**Priority areas for contributors:**
1. Auto-inject project health (1.1) - highest impact, clearest spec
2. Drift intervention (2.2) - low effort, high value
3. Reflection checkpoints (3.1) - UX design needed

---

## Feedback

This roadmap represents current thinking based on observed agent behavior and adoption challenges. It will evolve based on:
- Real-world usage data
- Community feedback
- Technical constraints discovered during implementation
- New insights from behavioral experiments

Join the discussion: [GitHub Discussions](#) or [Discord](#)
