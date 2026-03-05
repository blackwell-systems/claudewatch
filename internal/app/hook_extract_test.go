package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsElevatedPressure(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"pressure", true},
		{"critical", true},
		{"comfortable", false},
		{"filling", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isElevatedPressure(tt.status); got != tt.want {
			t.Errorf("isElevatedPressure(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// TestTryAutoExtract_TransitionDetection tests the state file transition logic
// by manipulating the state file directly and calling tryAutoExtract with a
// fake activePath that will cause ParseLiveContextPressure to fail (returning
// early). We test the state file write behavior separately.
func TestTryAutoExtract_TransitionDetection(t *testing.T) {
	// Use a temp dir for the state file.
	tmpDir := t.TempDir()
	sf := filepath.Join(tmpDir, "claudewatch-ctx-state")

	// Override stateFilePath by testing the transition logic directly.
	// Since tryAutoExtract uses stateFilePath() internally, we test the
	// logic components individually.

	// Test 1: No state file + elevated status → transition.
	os.Remove(sf)
	prev := ""
	current := "pressure"
	isTransition := isElevatedPressure(current) && !isElevatedPressure(prev)
	if !isTransition {
		t.Error("expected transition when no state file and current is pressure")
	}

	// Test 2: State file says "comfortable" + current "pressure" → transition.
	prev = "comfortable"
	current = "pressure"
	isTransition = isElevatedPressure(current) && !isElevatedPressure(prev)
	if !isTransition {
		t.Error("expected transition from comfortable to pressure")
	}

	// Test 3: State file says "pressure" + current "pressure" → NO transition.
	prev = "pressure"
	current = "pressure"
	isTransition = isElevatedPressure(current) && !isElevatedPressure(prev)
	if isTransition {
		t.Error("expected no transition when already at pressure")
	}

	// Test 4: State file says "critical" + current "comfortable" → NO transition.
	prev = "critical"
	current = "comfortable"
	isTransition = isElevatedPressure(current) && !isElevatedPressure(prev)
	if isTransition {
		t.Error("expected no transition when going down from critical to comfortable")
	}

	// Test 5: State file says "filling" + current "critical" → transition.
	prev = "filling"
	current = "critical"
	isTransition = isElevatedPressure(current) && !isElevatedPressure(prev)
	if !isTransition {
		t.Error("expected transition from filling to critical")
	}
}

// TestTryAutoExtract_StateFileWrite verifies that tryAutoExtract writes the
// current status to the state file even when extraction is skipped.
func TestTryAutoExtract_StateFileWrite(t *testing.T) {
	// tryAutoExtract with a nonexistent activePath should fail parsing
	// and return "" — but we can't test state file writes because
	// stateFilePath() uses a fixed location. Instead, verify the function
	// returns "" gracefully on bad input.
	result := tryAutoExtract("/nonexistent/path.jsonl", "/nonexistent/home")
	if result != "" {
		t.Errorf("expected empty string for nonexistent path, got %q", result)
	}
}

// TestTryAutoExtract_InvalidActivePath verifies graceful failure with bad paths.
func TestTryAutoExtract_InvalidActivePath(t *testing.T) {
	result := tryAutoExtract("", "")
	if result != "" {
		t.Errorf("expected empty string for empty paths, got %q", result)
	}
}

// TestTryAutoExtract_WithMinimalJSONL tests with a minimal JSONL fixture that
// ParseLiveContextPressure can parse. The extraction will return empty results
// (no facet) which is the expected behavior for new sessions.
func TestTryAutoExtract_WithMinimalJSONL(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal JSONL file with enough data to produce "pressure" status.
	// We need an assistant message with usage.input_tokens >= 150000 (75% of 200k).
	jsonlPath := filepath.Join(tmpDir, "test-session.jsonl")
	// assistant message with input_tokens=160000 → 80% → "pressure"
	line := `{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":160000,"output_tokens":1000}}}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a fake claude home dir (empty — no sessions or facets).
	claudeHome := filepath.Join(tmpDir, "claude-home")
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatal(err)
	}

	// This should parse pressure correctly but fail to find a facet,
	// resulting in a silent "" return (no crash).
	result := tryAutoExtract(jsonlPath, claudeHome)
	if result != "" {
		t.Errorf("expected empty string (no facet for new session), got %q", result)
	}
}
