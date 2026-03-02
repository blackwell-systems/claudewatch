package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLiveActiveTime_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.ActiveMinutes != 0 {
		t.Fatalf("expected ActiveMinutes=0, got %f", stats.ActiveMinutes)
	}
}

func TestParseLiveActiveTime_SingleEntry(t *testing.T) {
	ts := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage(ts, 1000, 500),
	})

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single entry — can't compute a duration.
	if stats.ActiveMinutes != 0 {
		t.Fatalf("expected ActiveMinutes=0 for single entry, got %f", stats.ActiveMinutes)
	}
}

func TestParseLiveActiveTime_ContinuousWork(t *testing.T) {
	// 5 entries, 1 minute apart — all active, no idle gaps.
	base := time.Now().Add(-5 * time.Minute).UTC()
	var entries []map[string]any
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		entries = append(entries, mkAssistantWithUsage(ts, 1000, 500))
	}
	path := writeLiveJSONL(t, entries)

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 4 minutes wall clock, 0 idle, 0 resumptions.
	if stats.Resumptions != 0 {
		t.Fatalf("expected Resumptions=0, got %d", stats.Resumptions)
	}
	if stats.IdleMinutes > 0.1 {
		t.Fatalf("expected IdleMinutes~0, got %f", stats.IdleMinutes)
	}
	if stats.ActiveMinutes < 3.9 || stats.ActiveMinutes > 4.1 {
		t.Fatalf("expected ActiveMinutes~4, got %f", stats.ActiveMinutes)
	}
}

func TestParseLiveActiveTime_WithIdleGap(t *testing.T) {
	// Work for 2 min, idle for 30 min, work for 2 min.
	base := time.Now().Add(-35 * time.Minute).UTC()
	entries := []map[string]any{
		mkAssistantWithUsage(base.Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(1*time.Minute).Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(2*time.Minute).Format(time.RFC3339), 1000, 500),
		// 30-minute gap here
		mkAssistantWithUsage(base.Add(32*time.Minute).Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(33*time.Minute).Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(34*time.Minute).Format(time.RFC3339), 1000, 500),
	}
	path := writeLiveJSONL(t, entries)

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wall clock: 34 min. Idle: ~30 min. Active: ~4 min. Resumptions: 1.
	if stats.Resumptions != 1 {
		t.Fatalf("expected Resumptions=1, got %d", stats.Resumptions)
	}
	if stats.IdleMinutes < 29 || stats.IdleMinutes > 31 {
		t.Fatalf("expected IdleMinutes~30, got %f", stats.IdleMinutes)
	}
	if stats.ActiveMinutes < 3 || stats.ActiveMinutes > 5 {
		t.Fatalf("expected ActiveMinutes~4, got %f", stats.ActiveMinutes)
	}
	if stats.WallClockMinutes < 33 || stats.WallClockMinutes > 35 {
		t.Fatalf("expected WallClockMinutes~34, got %f", stats.WallClockMinutes)
	}
}

func TestParseLiveActiveTime_MultipleResumptions(t *testing.T) {
	// 3 bursts of activity separated by 2 idle gaps.
	base := time.Now().Add(-70 * time.Minute).UTC()
	entries := []map[string]any{
		mkAssistantWithUsage(base.Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(1*time.Minute).Format(time.RFC3339), 1000, 500),
		// 20 min gap
		mkAssistantWithUsage(base.Add(21*time.Minute).Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(22*time.Minute).Format(time.RFC3339), 1000, 500),
		// 40 min gap
		mkAssistantWithUsage(base.Add(62*time.Minute).Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(63*time.Minute).Format(time.RFC3339), 1000, 500),
	}
	path := writeLiveJSONL(t, entries)

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Resumptions != 2 {
		t.Fatalf("expected Resumptions=2, got %d", stats.Resumptions)
	}
	// Active: ~3 min (1+1+1). Idle: ~60 min (20+40).
	if stats.ActiveMinutes < 2 || stats.ActiveMinutes > 4 {
		t.Fatalf("expected ActiveMinutes~3, got %f", stats.ActiveMinutes)
	}

	_ = fmt.Sprintf("") // keep fmt import for writeLiveJSONL's json.Marshal usage
}

func TestParseLiveActiveTime_ExactThreshold(t *testing.T) {
	// Gap of exactly 5 minutes — should NOT be idle (threshold is >5min).
	base := time.Now().Add(-10 * time.Minute).UTC()
	entries := []map[string]any{
		mkAssistantWithUsage(base.Format(time.RFC3339), 1000, 500),
		mkAssistantWithUsage(base.Add(5*time.Minute).Format(time.RFC3339), 1000, 500),
	}
	path := writeLiveJSONL(t, entries)

	stats, err := ParseLiveActiveTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Resumptions != 0 {
		t.Fatalf("expected Resumptions=0 for exactly 5min gap, got %d", stats.Resumptions)
	}
}

// writeLiveJSONL helper uses the one from active_live_test.go (same package).
// mkAssistantWithUsage helper uses the one from active_live_context_test.go.
// Both are accessible since this is in the same test package.

// Ensure json import is used (needed by writeLiveJSONL indirectly).
var _ = json.Marshal
