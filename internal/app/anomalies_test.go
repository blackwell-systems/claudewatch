package app

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestRenderAnomalies_Empty verifies that renderAnomalies does not panic
// when given an empty anomaly list.
func TestRenderAnomalies_Empty(t *testing.T) {
	baseline := store.ProjectBaseline{
		Project:      "test-project",
		SessionCount: 10,
		AvgCostUSD:   0.05,
		AvgFriction:  1.5,
	}

	// Should not panic.
	renderAnomalies(nil, "test-project", baseline, 2.0)
}

// TestRenderAnomalies_WithData verifies that renderAnomalies does not panic
// when given a populated anomaly list.
func TestRenderAnomalies_WithData(t *testing.T) {
	baseline := store.ProjectBaseline{
		Project:        "claudewatch",
		SessionCount:   20,
		AvgCostUSD:     0.10,
		StddevCostUSD:  0.03,
		AvgFriction:    2.0,
		StddevFriction: 0.8,
	}

	anomalies := []store.AnomalyResult{
		{
			SessionID:      "abc123def456xyz",
			Project:        "claudewatch",
			StartTime:      "2024-01-15T10:30:00Z",
			CostUSD:        0.25,
			Friction:       8,
			CostZScore:     3.2,
			FrictionZScore: 2.5,
			Severity:       "critical",
			Reason:         "high cost (z=3.20) and high friction (z=2.50)",
		},
		{
			SessionID:      "xyz789uvw012abc",
			Project:        "claudewatch",
			StartTime:      "2024-01-16T09:00:00Z",
			CostUSD:        0.19,
			Friction:       5,
			CostZScore:     2.1,
			FrictionZScore: 1.8,
			Severity:       "warning",
			Reason:         "high cost deviation (z=2.10)",
		},
	}

	// Should not panic.
	renderAnomalies(anomalies, "claudewatch", baseline, 2.0)
}

// TestAnomaliesCmd_Registered verifies that anomaliesCmd is registered on rootCmd.
func TestAnomaliesCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "anomalies" {
			return
		}
	}
	t.Fatal("anomalies subcommand not registered on rootCmd")
}

// TestAnomaliesFlagThreshold_DefaultValue verifies the default threshold flag value.
func TestAnomaliesFlagThreshold_DefaultValue(t *testing.T) {
	flag := anomaliesCmd.Flags().Lookup("threshold")
	if flag == nil {
		t.Fatal("expected --threshold flag to be registered on anomaliesCmd")
	}
	if flag.DefValue != "2" {
		t.Errorf("expected default value of --threshold to be %q, got %q", "2", flag.DefValue)
	}
}

// TestAnomaliesFlagProject_Registered verifies --project flag is registered on anomaliesCmd.
func TestAnomaliesFlagProject_Registered(t *testing.T) {
	flag := anomaliesCmd.Flags().Lookup("project")
	if flag == nil {
		t.Fatal("expected --project flag to be registered on anomaliesCmd")
	}
}

// TestCheckAnomalyBaselines_NoSessions verifies that the check passes vacuously
// when no sessions exist.
func TestCheckAnomalyBaselines_NoSessions(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	result := checkAnomalyBaselines(db, nil, nil)
	if !result.Passed {
		t.Errorf("expected check to pass vacuously with no sessions, got: %s", result.Message)
	}
}

// TestCheckAnomalyBaselines_FewSessions verifies the check passes when all projects
// have fewer than 5 sessions.
func TestCheckAnomalyBaselines_FewSessions(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/code/myproject"},
		{SessionID: "s2", ProjectPath: "/code/myproject"},
		{SessionID: "s3", ProjectPath: "/code/myproject"},
	}

	result := checkAnomalyBaselines(db, sessions, nil)
	if !result.Passed {
		t.Errorf("expected check to pass when project has <5 sessions, got: %s", result.Message)
	}
}

// TestCheckAnomalyBaselines_MissingBaseline verifies the check fails when a project
// with ≥5 sessions has no baseline.
func TestCheckAnomalyBaselines_MissingBaseline(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	sessions := make([]claude.SessionMeta, 5)
	for i := range sessions {
		sessions[i] = claude.SessionMeta{
			SessionID:   "s" + string(rune('0'+i)),
			ProjectPath: "/code/bigproject",
		}
	}

	result := checkAnomalyBaselines(db, sessions, nil)
	if result.Passed {
		t.Error("expected check to fail when project has ≥5 sessions but no baseline")
	}
}

// TestCheckAnomalyBaselines_BaselinePresent verifies the check passes when a project
// with ≥5 sessions has a stored baseline.
func TestCheckAnomalyBaselines_BaselinePresent(t *testing.T) {
	db, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("opening in-memory DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	sessions := make([]claude.SessionMeta, 5)
	for i := range sessions {
		sessions[i] = claude.SessionMeta{
			SessionID:   "s" + string(rune('0'+i)),
			ProjectPath: "/code/bigproject",
		}
	}

	// Store a baseline for the project.
	if err := db.UpsertProjectBaseline(store.ProjectBaseline{
		Project:      "bigproject",
		SessionCount: 5,
		AvgCostUSD:   0.05,
	}); err != nil {
		t.Fatalf("upserting baseline: %v", err)
	}

	result := checkAnomalyBaselines(db, sessions, nil)
	if !result.Passed {
		t.Errorf("expected check to pass when baseline exists, got: %s", result.Message)
	}
}
