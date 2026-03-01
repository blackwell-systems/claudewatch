Developers make changes to their CLAUDE.md files based on intuition. They add scope constraints, testing sections, tool guidance — and then hope things improve. claudewatch replaces hope with measurement. The effectiveness system compares session metrics before and after each CLAUDE.md change, producing a scored verdict that tells you whether your change actually helped.

## How it works

**Change detection.** claudewatch tracks each CLAUDE.md file's modification timestamp. When it changes, that timestamp becomes the boundary between "before" and "after" session groups.

**Metrics compared.** Five metrics are measured across sessions before the change and sessions after it:

| Metric | Weight | What it captures |
|---|---|---|
| Friction rate | 30 pts | Average friction events per session |
| Tool errors | 20 pts | Average tool errors per session |
| Interruptions | 20 pts | Average user interruptions per session |
| Goal achievement | 20 pts | Fraction of sessions with outcome `achieved` or `mostly_achieved` |
| Cost per commit | 10 pts | Average cost per git commit produced |

**Score and verdict.** Each metric's before/after delta is converted to a percentage change relative to the baseline, weighted, and summed. The result is a score from -100 to +100. Three verdicts map to score ranges:

| Verdict | Score range | Meaning |
|---|---|---|
| `effective` | 20 to 100 | The change produced measurable improvement |
| `neutral` | 0 to 19 | Metrics moved slightly but not enough to call it an improvement |
| `regression` | -100 to -1 | Friction, errors, or interruptions went up after the change |

For friction, tool errors, interruptions, and cost, a negative delta is an improvement (the metric went down). For goal achievement, a positive delta is an improvement (more sessions succeeded). The score reflects this directionally.

**Insufficient data.** The minimum to compute a verdict is two sessions on each side of the change timestamp. When either side has fewer than two sessions, the verdict is `insufficient_data`. This is not a failure — it means the change is too recent, or this project doesn't have enough sessions yet. Check back after a few more sessions.

## Reading the output

```
project:              shelfctl
verdict:              effective
score:                58
friction_delta:       -1.40    ← avg friction events/session dropped by 1.4
tool_error_delta:     -0.90    ← avg tool errors/session dropped by 0.9
interruption_delta:   -0.30    ← avg interruptions/session dropped by 0.3
goal_delta:           +0.22    ← goal achievement rate up 22 percentage points
cost_delta:           -0.48    ← cost per commit dropped by $0.48
before_sessions:      11
after_sessions:       19
change_detected:      2026-02-26
```

- **score**: -100 to +100. Scores at or above 20 indicate a meaningful improvement (`effective`). Scores below 0 indicate the change made things worse (`regression`).
- **friction_delta**: average friction events per session, after minus before. Negative means fewer friction events — an improvement.
- **tool_error_delta**: average tool errors per session, after minus before. Negative means fewer errors.
- **interruption_delta**: average user interruptions per session, after minus before. Negative means fewer interruptions.
- **goal_delta**: fraction of sessions with an achieved or mostly-achieved outcome, after minus before. Positive means more sessions succeeded.
- **cost_delta**: cost per git commit, after minus before. Negative means less spend per unit of output.
- **before_sessions / after_sessions**: how many sessions contributed to each side. Results with fewer than five sessions per side should be treated as indicative rather than conclusive — the minimum to produce any verdict is two per side, but more sessions reduce noise.
- **verdict**: human-readable summary of the score. `neutral` means the metrics moved but not significantly enough to call the change effective.

## Accessing effectiveness data

**CLI** (post-session analysis):

```bash
claudewatch metrics                        # includes effectiveness in standard output
claudewatch metrics --effectiveness        # effectiveness section only
claudewatch metrics --json | jq '.effectiveness'
```

**MCP** (in-session, real-time):

```
get_effectiveness   → all projects with a CLAUDE.md change and sufficient session data
```

The MCP tool is useful when deciding mid-session whether a previous CLAUDE.md edit is working. You can check without leaving the session, which makes it practical to treat effectiveness as a decision input rather than an after-the-fact report.

## Common patterns

**`insufficient_data` on both sides.** The CLAUDE.md was just changed, or this project doesn't have many sessions yet. Keep working and check back after 5–10 more sessions.

**`insufficient_data` on the after side only.** The change is recent and there hasn't been enough activity to measure it. Normal — give it time.

**`neutral` with small deltas.** The change probably didn't hurt, but it didn't address a significant friction source. Consider whether the friction type you targeted is still recurring — check `get_stale_patterns` to see if the pattern is still showing up in recent sessions.

**`regression`.** Friction, tool errors, or interruptions went up after the change. This can happen when CLAUDE.md additions conflict with how you actually work — overly restrictive scope constraints, for example, can increase interruptions as you push back on them mid-session. Review what changed and consider reverting or narrowing the scope of the addition.

**`effective` but still high friction.** The change helped, but there's more work to do. The score tells you the change moved things in the right direction, not that the project is fully optimized. Run `claudewatch suggest` to find the next highest-impact fix.

## The full loop

```
1. claudewatch suggest --project myproject   # find the highest-impact fix
2. claudewatch fix myproject                 # apply it
3. Work for several sessions
4. claudewatch metrics --effectiveness       # check whether it helped
   OR: get_effectiveness (MCP, mid-session)
5. Repeat
```

The goal is not a single perfect CLAUDE.md. It is a continuous improvement cycle where each change is measured rather than assumed. A `neutral` result is useful information — it tells you the change was harmless but that the friction you targeted either didn't exist at the rate you expected or isn't captured by the five scored metrics. An `effective` result tells you to keep the change and look for the next one. A `regression` result tells you to revert before it compounds.
