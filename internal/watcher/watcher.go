// Package watcher provides background monitoring of Claude Code session data,
// detecting friction spikes and notable changes and emitting alerts.
package watcher

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// WatchState captures a point-in-time snapshot of Claude session data.
type WatchState struct {
	Timestamp          time.Time
	SessionCount       int
	FrictionCounts     map[string]int // friction type -> count
	AgentCount         int
	AgentKillCount     int
	ZeroCommitCount    int
	TotalSessions      int
	StalePatterns      int
	LastSessionID      string
	EstimatedDailyCost float64 // estimated cost for today's sessions

	// Internal: keep richer data for comparison.
	frictionByType   map[string]int
	agentKillRate    float64
	agentSuccessRate float64
	persistence      analyzer.PersistenceAnalysis
	sessions         []claude.SessionMeta
	facets           []claude.SessionFacet
}

// Alert represents a notable event detected by the watcher.
type Alert struct {
	Level   string // "info", "warning", "critical"
	Title   string
	Message string
	Time    time.Time
}

// Watcher monitors Claude session data at a regular interval and emits alerts
// when notable changes are detected.
type Watcher struct {
	claudeDir     string
	interval      time.Duration
	previous      *WatchState
	alertFn       func(Alert)     // callback for emitting alerts
	lastAlertKeys map[string]bool // dedup: suppress repeated identical alerts
	BudgetUSD     float64         // daily cost budget; 0 means no budget alert
}

// New creates a Watcher that monitors the given Claude data directory.
func New(claudeDir string, interval time.Duration, alertFn func(Alert)) *Watcher {
	return &Watcher{
		claudeDir:     claudeDir,
		interval:      interval,
		alertFn:       alertFn,
		lastAlertKeys: make(map[string]bool),
	}
}

// Run starts the watch loop. It takes an initial snapshot, then checks at
// every interval. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	// Take the initial snapshot.
	initial, err := w.Snapshot()
	if err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	w.previous = initial

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			alerts := w.Check()
			for _, a := range alerts {
				if w.alertFn != nil {
					w.alertFn(a)
				}
			}
		}
	}
}

// Check performs a single check cycle: takes a new snapshot, compares against
// the previous state, updates the previous state, and returns any alerts.
// Identical alerts are suppressed until the underlying data changes.
func (w *Watcher) Check() []Alert {
	curr, err := w.Snapshot()
	if err != nil {
		return []Alert{{
			Level:   "warning",
			Title:   "Snapshot failed",
			Message: fmt.Sprintf("Could not read session data: %v", err),
			Time:    time.Now(),
		}}
	}

	var raw []Alert
	if w.previous != nil {
		raw = Compare(w.previous, curr)
	}

	// Budget alert: fires when today's estimated cost exceeds the threshold.
	if w.BudgetUSD > 0 && curr.EstimatedDailyCost > w.BudgetUSD {
		raw = append(raw, Alert{
			Level:   "warning",
			Title:   "Daily cost budget exceeded",
			Message: fmt.Sprintf("Estimated $%.2f today (budget: $%.2f)", curr.EstimatedDailyCost, w.BudgetUSD),
			Time:    time.Now(),
		})
	}

	// Deduplicate: suppress alerts with the same title+message as last cycle.
	currentKeys := make(map[string]bool, len(raw))
	var alerts []Alert
	for _, a := range raw {
		key := a.Level + ":" + a.Title + ":" + a.Message
		currentKeys[key] = true
		if !w.lastAlertKeys[key] {
			alerts = append(alerts, a)
		}
	}
	w.lastAlertKeys = currentKeys

	w.previous = curr
	return alerts
}

// Snapshot captures the current state from Claude data. It reads session meta,
// facets, and agent tasks, computing summary counts. For efficiency, it checks
// whether the session-meta directory has been modified before doing a full parse.
func (w *Watcher) Snapshot() (*WatchState, error) {
	state := &WatchState{
		Timestamp:      time.Now(),
		FrictionCounts: make(map[string]int),
		frictionByType: make(map[string]int),
	}

	// Parse session metadata.
	sessions, err := claude.ParseAllSessionMeta(w.claudeDir)
	if err != nil {
		return nil, fmt.Errorf("parsing session meta: %w", err)
	}
	state.sessions = sessions
	state.SessionCount = len(sessions)
	state.TotalSessions = len(sessions)

	// Track the most recent session by start time.
	if len(sessions) > 0 {
		latest := sessions[0]
		for _, s := range sessions[1:] {
			if s.StartTime > latest.StartTime {
				latest = s
			}
		}
		state.LastSessionID = latest.SessionID
	}

	// Count zero-commit sessions.
	for _, s := range sessions {
		if s.GitCommits == 0 {
			state.ZeroCommitCount++
		}
	}

	// Parse facets for friction data.
	facets, err := claude.ParseAllFacets(w.claudeDir)
	if err != nil {
		// Non-fatal: friction data may not exist yet.
		facets = nil
	}
	state.facets = facets

	for _, f := range facets {
		for frictionType, count := range f.FrictionCounts {
			state.FrictionCounts[frictionType] += count
			state.frictionByType[frictionType] += count
		}
	}

	// Parse agent tasks.
	agentTasks, err := claude.ParseAgentTasks(w.claudeDir)
	if err != nil {
		// Non-fatal: transcript data may not exist.
		agentTasks = nil
	}
	state.AgentCount = len(agentTasks)

	for _, t := range agentTasks {
		if t.Status == "killed" {
			state.AgentKillCount++
		}
	}

	if len(agentTasks) > 0 {
		agentPerf := analyzer.AnalyzeAgents(agentTasks)
		state.agentKillRate = agentPerf.KillRate
		state.agentSuccessRate = agentPerf.SuccessRate
	}

	// Analyze friction persistence for stale pattern detection.
	if len(facets) > 0 && len(sessions) > 0 {
		persistence := analyzer.AnalyzeFrictionPersistence(facets, sessions)
		state.StalePatterns = persistence.StaleCount
		state.persistence = persistence
	}

	// Estimate today's cost from sessions starting today.
	// Load stats-cache for accurate cache-aware pricing (non-fatal if missing).
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(w.claudeDir); scErr == nil && sc != nil {
		cacheRatio = analyzer.ComputeCacheRatio(*sc)
	}
	today := time.Now().Format("2006-01-02")
	for _, s := range sessions {
		if len(s.StartTime) >= 10 && s.StartTime[:10] == today {
			state.EstimatedDailyCost += analyzer.EstimateSessionCost(s, pricing, cacheRatio)
		}
	}

	return state, nil
}

// recentSessions returns sessions sorted by start time descending, limited to n.
func recentSessions(sessions []claude.SessionMeta, n int) []claude.SessionMeta {
	if len(sessions) == 0 {
		return nil
	}

	sorted := make([]claude.SessionMeta, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime > sorted[j].StartTime
	})

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}
