package analyzer

import (
	"math"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// FrictionPersistence tracks how a single friction type persists across sessions over time.
type FrictionPersistence struct {
	// FrictionType is the friction category (e.g., "wrong_approach", "misunderstood_request").
	FrictionType string `json:"friction_type"`

	// FirstSeen is the earliest session timestamp where this friction appeared.
	FirstSeen time.Time `json:"first_seen"`

	// LastSeen is the most recent session timestamp where this friction appeared.
	LastSeen time.Time `json:"last_seen"`

	// OccurrenceCount is how many sessions this friction type appeared in.
	OccurrenceCount int `json:"occurrence_count"`

	// TotalSessions is the total number of sessions in the analysis window.
	TotalSessions int `json:"total_sessions"`

	// Frequency is the ratio of OccurrenceCount to TotalSessions.
	Frequency float64 `json:"frequency"`

	// WeeklyTrend indicates the direction of change: "improving", "stable", or "worsening".
	WeeklyTrend string `json:"weekly_trend"`

	// ConsecutiveWeeks is how many consecutive weeks this friction type appeared.
	ConsecutiveWeeks int `json:"consecutive_weeks"`

	// Stale is true when the friction has appeared for 3+ consecutive weeks without improving.
	Stale bool `json:"stale"`
}

// PersistenceAnalysis is the result of analyzing friction persistence across sessions.
type PersistenceAnalysis struct {
	// Patterns contains persistence data for each observed friction type.
	Patterns []FrictionPersistence `json:"patterns"`

	// StaleCount is the number of patterns present 3+ weeks without improving.
	StaleCount int `json:"stale_count"`

	// ImprovingCount is the number of patterns trending downward.
	ImprovingCount int `json:"improving_count"`

	// WorseningCount is the number of patterns trending upward.
	WorseningCount int `json:"worsening_count"`
}

// weekKey returns the ISO year and week number for a given time, used as a bucket key.
func weekKey(t time.Time) [2]int {
	year, week := t.ISOWeek()
	return [2]int{year, week}
}

// weeksBetween returns a sorted slice of all week keys between the earliest and latest
// times (inclusive), covering the full analysis window without gaps.
func weeksBetween(earliest, latest time.Time) [][2]int {
	if earliest.After(latest) {
		return nil
	}

	var weeks [][2]int
	seen := make(map[[2]int]bool)

	// Walk forward day-by-day from earliest to latest to capture every week boundary.
	for t := earliest; !t.After(latest); t = t.AddDate(0, 0, 1) {
		wk := weekKey(t)
		if !seen[wk] {
			seen[wk] = true
			weeks = append(weeks, wk)
		}
	}

	// Ensure we include the final week.
	wk := weekKey(latest)
	if !seen[wk] {
		weeks = append(weeks, wk)
	}

	return weeks
}

// computeTrend compares the average count in the last 2 weeks vs the prior 2 weeks.
// With fewer than 4 weeks of data it uses whatever is available, splitting at the midpoint.
// Returns "improving", "stable", or "worsening".
func computeTrend(weeklyCounts []int) string {
	n := len(weeklyCounts)
	if n < 2 {
		return "stable"
	}

	// Split into earlier half and later half.
	// With 4+ weeks: prior 2 vs last 2.
	// With fewer: split at midpoint.
	splitAt := n - 2
	if splitAt < 1 {
		splitAt = 1
	}

	earlierSum := 0
	for i := 0; i < splitAt && i < len(weeklyCounts); i++ {
		earlierSum += weeklyCounts[i]
	}
	earlierAvg := float64(earlierSum) / float64(splitAt)

	laterLen := n - splitAt
	laterSum := 0
	for i := splitAt; i < n; i++ {
		laterSum += weeklyCounts[i]
	}
	laterAvg := float64(laterSum) / float64(laterLen)

	if earlierAvg == 0 && laterAvg == 0 {
		return "stable"
	}
	if earlierAvg == 0 {
		return "worsening"
	}

	change := (laterAvg - earlierAvg) / earlierAvg

	switch {
	case change < -0.10:
		return "improving"
	case change > 0.10:
		return "worsening"
	default:
		return "stable"
	}
}

// consecutiveWeeksFromEnd counts how many consecutive weeks at the end of the
// allWeeks slice had at least one occurrence.
func consecutiveWeeksFromEnd(allWeeks [][2]int, weekPresence map[[2]int]bool) int {
	count := 0
	for i := len(allWeeks) - 1; i >= 0; i-- {
		if weekPresence[allWeeks[i]] {
			count++
		} else {
			break
		}
	}
	return count
}

// AnalyzeFrictionPersistence examines whether friction patterns persist across
// sessions over time. It correlates facets with session metadata to obtain
// timestamps, then buckets friction occurrences into weekly bins to compute
// trends and staleness.
//
// Sessions in facets that have no matching entry in metas (and thus no timestamp)
// are excluded from the analysis.
func AnalyzeFrictionPersistence(facets []claude.SessionFacet, metas []claude.SessionMeta) PersistenceAnalysis {
	result := PersistenceAnalysis{}

	if len(facets) == 0 {
		return result
	}

	// Build session-id to start-time lookup from session metadata.
	sessionTime := make(map[string]time.Time, len(metas))
	for _, m := range metas {
		if m.SessionID == "" || m.StartTime == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, m.StartTime)
		if err != nil {
			// Try alternative formats that Claude Code might use.
			t, err = time.Parse("2006-01-02T15:04:05", m.StartTime)
			if err != nil {
				continue
			}
		}
		sessionTime[m.SessionID] = t
	}

	// Pair each facet with its timestamp, skipping those without metadata.
	type timedFacet struct {
		facet claude.SessionFacet
		ts    time.Time
	}
	var timed []timedFacet
	for _, f := range facets {
		ts, ok := sessionTime[f.SessionID]
		if !ok {
			continue
		}
		timed = append(timed, timedFacet{facet: f, ts: ts})
	}

	totalSessions := len(timed)
	if totalSessions == 0 {
		return result
	}

	// Sort by timestamp so week calculations are ordered.
	sort.Slice(timed, func(i, j int) bool {
		return timed[i].ts.Before(timed[j].ts)
	})

	earliest := timed[0].ts
	latest := timed[len(timed)-1].ts
	allWeeks := weeksBetween(earliest, latest)

	// For each friction type, collect: which weeks it appeared in, session count,
	// first/last seen.
	type frictionData struct {
		weekCounts   map[[2]int]int
		weekPresence map[[2]int]bool
		sessionCount int
		firstSeen    time.Time
		lastSeen     time.Time
	}
	byType := make(map[string]*frictionData)

	for _, tf := range timed {
		if len(tf.facet.FrictionCounts) == 0 {
			continue
		}
		wk := weekKey(tf.ts)
		for frictionType := range tf.facet.FrictionCounts {
			fd, ok := byType[frictionType]
			if !ok {
				fd = &frictionData{
					weekCounts:   make(map[[2]int]int),
					weekPresence: make(map[[2]int]bool),
					firstSeen:    tf.ts,
					lastSeen:     tf.ts,
				}
				byType[frictionType] = fd
			}
			fd.weekCounts[wk]++
			fd.weekPresence[wk] = true
			fd.sessionCount++
			if tf.ts.Before(fd.firstSeen) {
				fd.firstSeen = tf.ts
			}
			if tf.ts.After(fd.lastSeen) {
				fd.lastSeen = tf.ts
			}
		}
	}

	// Build persistence entries for each friction type.
	for frictionType, fd := range byType {
		// Build ordered weekly count slice aligned to allWeeks.
		weeklyCounts := make([]int, len(allWeeks))
		for i, wk := range allWeeks {
			weeklyCounts[i] = fd.weekCounts[wk]
		}

		trend := computeTrend(weeklyCounts)
		consec := consecutiveWeeksFromEnd(allWeeks, fd.weekPresence)
		freq := float64(fd.sessionCount) / float64(totalSessions)
		stale := consec >= 3 && trend != "improving"

		p := FrictionPersistence{
			FrictionType:     frictionType,
			FirstSeen:        fd.firstSeen,
			LastSeen:         fd.lastSeen,
			OccurrenceCount:  fd.sessionCount,
			TotalSessions:    totalSessions,
			Frequency:        math.Round(freq*1000) / 1000, // Round to 3 decimal places.
			WeeklyTrend:      trend,
			ConsecutiveWeeks: consec,
			Stale:            stale,
		}
		result.Patterns = append(result.Patterns, p)

		switch {
		case stale:
			result.StaleCount++
		case trend == "improving":
			result.ImprovingCount++
		case trend == "worsening":
			result.WorseningCount++
		}
	}

	// Sort: stale first, then by frequency descending.
	sort.Slice(result.Patterns, func(i, j int) bool {
		pi, pj := result.Patterns[i], result.Patterns[j]
		if pi.Stale != pj.Stale {
			return pi.Stale
		}
		return pi.Frequency > pj.Frequency
	})

	return result
}
