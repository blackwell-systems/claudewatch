package watcher

import (
	"testing"
	"time"
)

func TestNotify_DoesNotPanic(t *testing.T) {
	tests := []struct {
		name  string
		alert Alert
	}{
		{
			name: "info alert",
			alert: Alert{
				Level:   "info",
				Title:   "Session completed",
				Message: "10min, 2 commits",
				Time:    time.Now(),
			},
		},
		{
			name: "warning alert",
			alert: Alert{
				Level:   "warning",
				Title:   "Friction spike",
				Message: "wrong_approach increased by 50%",
				Time:    time.Now(),
			},
		},
		{
			name: "critical alert",
			alert: Alert{
				Level:   "critical",
				Title:   "Stale friction",
				Message: "Persisted for 4 weeks",
				Time:    time.Now(),
			},
		},
		{
			name: "empty fields",
			alert: Alert{
				Level:   "",
				Title:   "",
				Message: "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Notify should not panic regardless of input.
			// It may use osascript or fall back to stderr.
			err := Notify(tc.alert)
			// We don't check the error because it depends on the environment
			// (osascript availability, etc.). We just verify no panic.
			_ = err
		})
	}
}

func TestNotifyFallback_WritesToStderr(t *testing.T) {
	alert := Alert{
		Level:   "info",
		Title:   "Test alert",
		Message: "Test message",
		Time:    time.Now(),
	}

	// notifyFallback writes to stderr, which is fine for tests.
	err := notifyFallback(alert)
	if err != nil {
		t.Errorf("unexpected error from notifyFallback: %v", err)
	}
}
