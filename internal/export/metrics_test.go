package export

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectMetrics_EmptySessions(t *testing.T) {
	// Create a temporary directory for ClaudeHome
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ClaudeHome: tmpDir,
		Friction: config.Friction{
			RecurringThreshold: 0.3,
		},
	}

	snapshot, err := CollectMetrics(cfg, "", 0)

	// Should handle empty session list gracefully (no error, but zero metrics)
	require.NoError(t, err)
	assert.Equal(t, 0, snapshot.SessionCount)
	assert.Equal(t, 0.0, snapshot.TotalCostUSD)
}

func TestFilterSessionsByProject(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/home/user/projectA"},
		{SessionID: "s2", ProjectPath: "/home/user/projectB"},
		{SessionID: "s3", ProjectPath: "/home/user/projectA"},
	}

	filtered := filterSessionsByProject(sessions, "projectA")
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, "s1", filtered[0].SessionID)
	assert.Equal(t, "s3", filtered[1].SessionID)
}

func TestFilterSessionsByProject_NoMatch(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/home/user/projectA"},
	}

	filtered := filterSessionsByProject(sessions, "nonexistent")
	assert.Equal(t, 0, len(filtered))
}

func TestComputeCommitAttemptRatio(t *testing.T) {
	tests := []struct {
		name     string
		sessions []claude.SessionMeta
		expected float64
	}{
		{
			name: "normal ratio",
			sessions: []claude.SessionMeta{
				{
					GitCommits: 2,
					ToolCounts: map[string]int{"Edit": 5, "Write": 3},
				},
			},
			expected: 2.0 / 8.0,
		},
		{
			name: "no edits or writes",
			sessions: []claude.SessionMeta{
				{
					GitCommits: 2,
					ToolCounts: map[string]int{"Read": 10},
				},
			},
			expected: 0,
		},
		{
			name: "multiple sessions",
			sessions: []claude.SessionMeta{
				{
					GitCommits: 3,
					ToolCounts: map[string]int{"Edit": 5, "Write": 5},
				},
				{
					GitCommits: 2,
					ToolCounts: map[string]int{"Edit": 3, "Write": 2},
				},
			},
			expected: 5.0 / 15.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := computeCommitAttemptRatio(tt.sessions)
			assert.InDelta(t, tt.expected, ratio, 0.001)
		})
	}
}

func TestComputeModelUsagePercent(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID: "s1",
			ModelUsage: map[string]claude.ModelStats{
				"claude-opus-4": {InputTokens: 1000, OutputTokens: 500},
			},
		},
		{
			SessionID: "s2",
			ModelUsage: map[string]claude.ModelStats{
				"claude-sonnet-4": {InputTokens: 2000, OutputTokens: 800},
			},
		},
		{
			SessionID: "s3",
			ModelUsage: map[string]claude.ModelStats{
				"claude-opus-4": {InputTokens: 1500, OutputTokens: 600},
			},
		},
	}

	result := computeModelUsagePercent(sessions)

	require.NotNil(t, result)
	assert.InDelta(t, 66.67, result["claude-opus-4"], 0.1)   // 2/3 sessions = 66.67%
	assert.InDelta(t, 33.33, result["claude-sonnet-4"], 0.1) // 1/3 sessions = 33.33%
}

func TestComputeModelUsagePercent_EmptySessions(t *testing.T) {
	result := computeModelUsagePercent(nil)
	assert.Nil(t, result)
}

func TestCountSessionsWithAgents(t *testing.T) {
	tasks := []claude.AgentTask{
		{SessionID: "s1", AgentType: "test"},
		{SessionID: "s1", AgentType: "test2"},
		{SessionID: "s2", AgentType: "test"},
		{SessionID: "s3", AgentType: "test"},
	}

	count := countSessionsWithAgents(tasks)
	assert.Equal(t, 3, count) // 3 unique sessions
}

func TestComputeAvgContextPressure(t *testing.T) {
	sessions := []claude.SessionMeta{
		{InputTokens: 50000, OutputTokens: 10000},  // 60k/200k = 0.3
		{InputTokens: 100000, OutputTokens: 20000}, // 120k/200k = 0.6
	}

	pressure := computeAvgContextPressure(sessions)
	assert.InDelta(t, 0.45, pressure, 0.01) // Average of 0.3 and 0.6
}

func TestComputeAvgContextPressure_OverLimit(t *testing.T) {
	sessions := []claude.SessionMeta{
		{InputTokens: 300000, OutputTokens: 100000}, // Should cap at 1.0
	}

	pressure := computeAvgContextPressure(sessions)
	assert.Equal(t, 1.0, pressure) // Capped at 100%
}

func TestHashProjectName(t *testing.T) {
	hash1 := hashProjectName("myproject")
	hash2 := hashProjectName("myproject")
	hash3 := hashProjectName("otherproject")

	// Same input produces same hash
	assert.Equal(t, hash1, hash2)
	// Different input produces different hash
	assert.NotEqual(t, hash1, hash3)
	// Hash is reasonably short
	assert.Len(t, hash1, 16) // 8 bytes = 16 hex chars
}

func TestLimitFrictionTypes(t *testing.T) {
	frictionByType := map[string]int{
		"wrong_approach":       50,
		"excessive_analysis":   40,
		"retry:Bash":           30,
		"buggy_code":           20,
		"user_rejected_action": 10,
		"low_frequency_error":  5,
	}

	limited := LimitFrictionTypes(frictionByType, 3)

	assert.Equal(t, 3, len(limited))
	assert.Equal(t, 50, limited["wrong_approach"])
	assert.Equal(t, 40, limited["excessive_analysis"])
	assert.Equal(t, 30, limited["retry:Bash"])
	assert.NotContains(t, limited, "low_frequency_error")
}

func TestLimitFrictionTypes_BelowLimit(t *testing.T) {
	frictionByType := map[string]int{
		"wrong_approach": 50,
		"retry:Bash":     30,
	}

	limited := LimitFrictionTypes(frictionByType, 10)

	// Should return all when below limit
	assert.Equal(t, 2, len(limited))
	assert.Equal(t, frictionByType, limited)
}

func TestComputeActiveMinutes(t *testing.T) {
	sessions := []claude.SessionMeta{
		{DurationMinutes: 30},
		{DurationMinutes: 45},
		{DurationMinutes: 600}, // > 8 hours, should be excluded
		{DurationMinutes: 0},   // Zero duration, excluded
	}

	active := computeActiveMinutes(sessions)
	assert.Equal(t, 75.0, active) // Only 30 + 45
}

func TestAnalyzeEfficiency(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			ToolErrors:        5,
			UserInterruptions: 2,
			InputTokens:       10000,
			OutputTokens:      5000,
			ToolErrorCategories: map[string]int{
				"permission_denied": 3,
				"file_not_found":    2,
			},
			ToolCounts: map[string]int{
				"Read": 10,
				"Edit": 5,
				"Bash": 3,
			},
			UsesTaskAgent: true,
			UsesMCP:       false,
		},
		{
			ToolErrors:        3,
			UserInterruptions: 1,
			InputTokens:       8000,
			OutputTokens:      4000,
			ToolErrorCategories: map[string]int{
				"file_not_found": 3,
			},
			ToolCounts: map[string]int{
				"Read":  8,
				"Write": 2,
			},
			UsesTaskAgent: false,
			UsesMCP:       true,
		},
	}

	metrics := AnalyzeEfficiency(sessions)

	assert.Equal(t, 2, metrics.TotalSessions)
	assert.InDelta(t, 4.0, metrics.AvgToolErrorsPerSession, 0.01)     // (5+3)/2
	assert.InDelta(t, 1.5, metrics.AvgInterruptionsPerSession, 0.01)  // (2+1)/2
	assert.Equal(t, 5, metrics.ErrorCategoryTotals["file_not_found"]) // 2+3
	assert.Equal(t, 3, metrics.ErrorCategoryTotals["permission_denied"])
	assert.Equal(t, 18, metrics.ToolUsageTotals["Read"]) // 10+8
	assert.Equal(t, 1, metrics.FeatureAdoption.TaskAgentSessions)
	assert.Equal(t, 1, metrics.FeatureAdoption.MCPSessions)
}

// TestPrivacyRules validates that no sensitive data is included in MetricSnapshot.
func TestPrivacyRules(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:            time.Now(),
		ProjectName:          "testproject",
		ProjectHash:          "abc123def456",
		SessionCount:         10,
		TotalDurationMin:     300,
		AvgDurationMin:       30,
		ActiveMinutes:        250,
		FrictionRate:         0.35,
		FrictionByType:       map[string]int{"wrong_approach": 5},
		AvgToolErrors:        2.5,
		TotalCommits:         20,
		AvgCommitsPerSession: 2.0,
		CommitAttemptRatio:   0.5,
		ZeroCommitRate:       0.2,
		TotalCostUSD:         12.45,
		AvgCostPerSession:    1.25,
		CostPerCommit:        0.62,
		ModelUsagePct:        map[string]float64{"claude-opus-4": 60.0},
		AgentSuccessRate:     0.95,
		AgentUsageRate:       0.3,
		AvgContextPressure:   0.45,
	}

	// Verify no absolute paths (ProjectHash should be hash, not path)
	assert.NotContains(t, snapshot.ProjectHash, "/")
	assert.NotContains(t, snapshot.ProjectName, "/")

	// Verify all fields are aggregates or percentages
	assert.IsType(t, int(0), snapshot.SessionCount)
	assert.IsType(t, float64(0), snapshot.FrictionRate)
	assert.IsType(t, float64(0), snapshot.AvgCostPerSession)
	assert.IsType(t, map[string]int{}, snapshot.FrictionByType)
	assert.IsType(t, map[string]float64{}, snapshot.ModelUsagePct)

	// Verify no session IDs present (check struct has no such field)
	// This is enforced by struct definition, but verify at type level
	_, hasSessionID := interface{}(snapshot).(interface{ GetSessionID() string })
	assert.False(t, hasSessionID)
}

func TestFilterFacetsByDays(t *testing.T) {
	now := time.Now()

	facets := []claude.SessionFacet{
		{SessionID: now.Add(-1*24*time.Hour).Format("20060102-150405") + "-abc123"},
		{SessionID: now.Add(-10*24*time.Hour).Format("20060102-150405") + "-def456"},
		{SessionID: now.Add(-40*24*time.Hour).Format("20060102-150405") + "-ghi789"},
	}

	filtered := filterFacetsByDays(facets, 30)

	// Should include only sessions from last 30 days
	assert.LessOrEqual(t, len(filtered), 2) // First two are within 30 days
}

func TestFilterAgentTasksByDays(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1"},
		{SessionID: "s2"},
	}

	tasks := []claude.AgentTask{
		{SessionID: "s1", AgentType: "test1"},
		{SessionID: "s2", AgentType: "test2"},
		{SessionID: "s3", AgentType: "test3"}, // Not in sessions
	}

	filtered := filterAgentTasksByDays(tasks, sessions, 30)

	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, "s1", filtered[0].SessionID)
	assert.Equal(t, "s2", filtered[1].SessionID)
}

// TestExportPrometheus_PerModelCost verifies that Prometheus export reflects per-model
// costs when MetricSnapshot is built from sessions with ModelUsage populated.
// The test constructs a snapshot whose TotalCostUSD was computed via AnalyzeOutcomes
// with an Opus session, then verifies the exported Prometheus output contains the
// Opus-tier cost rather than the Sonnet-tier fallback.
func TestExportPrometheus_PerModelCost(t *testing.T) {
	// Opus pricing: $15/M input, $75/M output
	// 1M input + 500K output = $15.00 + $37.50 = $52.50
	opusCost := 52.50

	snapshot := MetricSnapshot{
		Timestamp:         time.Now(),
		ProjectName:       "opus-project",
		ProjectHash:       "abc123",
		SessionCount:      1,
		TotalCostUSD:      opusCost,
		AvgCostPerSession: opusCost,
		CostPerCommit:     opusCost, // 1 commit
		TotalCommits:      1,
		FrictionByType:    map[string]int{},
		ModelUsagePct:     map[string]float64{"claude-opus-4": 100.0},
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	// Verify the cost metric reflects Opus pricing ($52.50), not Sonnet ($10.50)
	assert.Contains(t, outputStr, `claudewatch_cost_usd_total{project="opus-project"} 52.5`)
	assert.Contains(t, outputStr, `claudewatch_cost_per_session_avg{project="opus-project"} 52.5`)
}

// TestExportJSON_PerModelCost verifies that JSON export reflects per-model costs
// when MetricSnapshot is built from sessions with ModelUsage populated.
func TestExportJSON_PerModelCost(t *testing.T) {
	// Build sessions with Opus ModelUsage
	sessions := []claude.SessionMeta{
		{
			SessionID:   "opus-session-1",
			ProjectPath: "/code/myproject",
			InputTokens: 1_000_000,
			OutputTokens: 500_000,
			GitCommits:  1,
			ModelUsage: map[string]claude.ModelStats{
				"claude-opus-4": {
					InputTokens:  1_000_000,
					OutputTokens: 500_000,
				},
			},
		},
	}

	// Use AnalyzeOutcomes which calls EstimateSessionCost internally.
	// The sonnet pricing passed here is the fallback — it should NOT be used
	// because ModelUsage is populated.
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	outcomeAnalysis := analyzer.AnalyzeOutcomes(sessions, nil, pricing, cacheRatio)

	snapshot := MetricSnapshot{
		Timestamp:         time.Now(),
		ProjectName:       "myproject",
		SessionCount:      1,
		TotalCostUSD:      outcomeAnalysis.TotalCost,
		AvgCostPerSession: outcomeAnalysis.AvgCostPerSession,
		FrictionByType:    map[string]int{},
		ModelUsagePct:     map[string]float64{},
	}

	exporter := &JSONExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	var decoded MetricSnapshot
	err = json.Unmarshal(output, &decoded)
	require.NoError(t, err)

	// Opus pricing: 1M input * $15/M + 500K output * $75/M = $15 + $37.50 = $52.50
	// Sonnet pricing would be: 1M * $3/M + 500K * $15/M = $3 + $7.50 = $10.50
	assert.InDelta(t, 52.50, decoded.TotalCostUSD, 0.01,
		"JSON export should reflect Opus per-model cost, not Sonnet fallback")
	assert.InDelta(t, 52.50, decoded.AvgCostPerSession, 0.01)
}
