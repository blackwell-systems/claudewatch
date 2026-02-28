package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestAnalyzeTodos_Empty(t *testing.T) {
	result := AnalyzePlanning(nil, nil)
	if result.Todos.TotalTasks != 0 {
		t.Errorf("expected 0 tasks, got %d", result.Todos.TotalTasks)
	}
}

func TestAnalyzeTodos_BasicCounts(t *testing.T) {
	todos := []claude.SessionTodos{
		{
			SessionID: "s1",
			Tasks: []claude.TodoTask{
				{Content: "Fix bug", Status: "completed"},
				{Content: "Add test", Status: "completed"},
				{Content: "Deploy", Status: "pending"},
			},
		},
		{
			SessionID: "s2",
			Tasks: []claude.TodoTask{
				{Content: "Refactor", Status: "in_progress"},
				{Content: "Review", Status: "pending"},
			},
		},
	}

	result := AnalyzePlanning(todos, nil)
	ta := result.Todos

	if ta.TotalTasks != 5 {
		t.Errorf("TotalTasks = %d, want 5", ta.TotalTasks)
	}
	if ta.CompletedTasks != 2 {
		t.Errorf("CompletedTasks = %d, want 2", ta.CompletedTasks)
	}
	if ta.PendingTasks != 2 {
		t.Errorf("PendingTasks = %d, want 2", ta.PendingTasks)
	}
	if ta.InProgressTasks != 1 {
		t.Errorf("InProgressTasks = %d, want 1", ta.InProgressTasks)
	}
	if ta.SessionsWithTodos != 2 {
		t.Errorf("SessionsWithTodos = %d, want 2", ta.SessionsWithTodos)
	}

	// 2/5 = 0.4
	wantRate := 0.4
	if diff := ta.CompletionRate - wantRate; diff > 0.01 || diff < -0.01 {
		t.Errorf("CompletionRate = %.2f, want %.2f", ta.CompletionRate, wantRate)
	}

	// 5/2 = 2.5
	wantAvg := 2.5
	if diff := ta.AvgTasksPerSession - wantAvg; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgTasksPerSession = %.2f, want %.2f", ta.AvgTasksPerSession, wantAvg)
	}
}

func TestAnalyzeFileChurn_Empty(t *testing.T) {
	result := AnalyzePlanning(nil, nil)
	if result.FileChurn.TotalSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", result.FileChurn.TotalSessions)
	}
}

func TestAnalyzeFileChurn_BasicMetrics(t *testing.T) {
	history := []claude.FileHistorySession{
		{SessionID: "s1", UniqueFiles: 10, TotalEdits: 30, MaxVersion: 5, TotalBytes: 50000},
		{SessionID: "s2", UniqueFiles: 5, TotalEdits: 8, MaxVersion: 3, TotalBytes: 10000},
	}

	result := AnalyzePlanning(nil, history)
	fc := result.FileChurn

	if fc.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", fc.TotalSessions)
	}
	if fc.TotalFiles != 15 {
		t.Errorf("TotalFiles = %d, want 15", fc.TotalFiles)
	}
	if fc.TotalEdits != 38 {
		t.Errorf("TotalEdits = %d, want 38", fc.TotalEdits)
	}
	if fc.MaxSessionEdits != 30 {
		t.Errorf("MaxSessionEdits = %d, want 30", fc.MaxSessionEdits)
	}

	// 38/15 ≈ 2.53
	wantEditsPerFile := 38.0 / 15.0
	if diff := fc.AvgEditsPerFile - wantEditsPerFile; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgEditsPerFile = %.2f, want %.2f", fc.AvgEditsPerFile, wantEditsPerFile)
	}

	// 15/2 = 7.5
	wantFilesPerSession := 7.5
	if diff := fc.AvgFilesPerSession - wantFilesPerSession; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgFilesPerSession = %.2f, want %.2f", fc.AvgFilesPerSession, wantFilesPerSession)
	}
}
