package analyzer

import (
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// TodoAnalysis captures aggregate metrics about Claude Code task planning usage.
type TodoAnalysis struct {
	TotalTasks         int     `json:"total_tasks"`
	CompletedTasks     int     `json:"completed_tasks"`
	PendingTasks       int     `json:"pending_tasks"`
	InProgressTasks    int     `json:"in_progress_tasks"`
	CompletionRate     float64 `json:"completion_rate"`
	SessionsWithTodos  int     `json:"sessions_with_todos"`
	AvgTasksPerSession float64 `json:"avg_tasks_per_session"`
}

// FileChurnAnalysis captures file editing intensity from file-history data.
type FileChurnAnalysis struct {
	TotalSessions      int     `json:"total_sessions"`
	TotalFiles         int     `json:"total_files"`
	TotalEdits         int     `json:"total_edits"`
	AvgEditsPerFile    float64 `json:"avg_edits_per_file"`
	AvgFilesPerSession float64 `json:"avg_files_per_session"`
	MaxSessionEdits    int     `json:"max_session_edits"`
	TotalBytes         int64   `json:"total_bytes"`
}

// PlanningAnalysis combines todo and file-churn metrics.
type PlanningAnalysis struct {
	Todos     TodoAnalysis      `json:"todos"`
	FileChurn FileChurnAnalysis `json:"file_churn"`
}

// AnalyzePlanning computes task planning and file churn metrics from
// todos and file-history data.
func AnalyzePlanning(todos []claude.SessionTodos, fileHistory []claude.FileHistorySession) PlanningAnalysis {
	return PlanningAnalysis{
		Todos:     analyzeTodos(todos),
		FileChurn: analyzeFileChurn(fileHistory),
	}
}

func analyzeTodos(todos []claude.SessionTodos) TodoAnalysis {
	if len(todos) == 0 {
		return TodoAnalysis{}
	}

	var total, completed, pending, inProgress int
	for _, st := range todos {
		for _, t := range st.Tasks {
			total++
			switch t.Status {
			case "completed":
				completed++
			case "pending":
				pending++
			case "in_progress":
				inProgress++
			}
		}
	}

	var completionRate float64
	if total > 0 {
		completionRate = float64(completed) / float64(total)
	}

	return TodoAnalysis{
		TotalTasks:         total,
		CompletedTasks:     completed,
		PendingTasks:       pending,
		InProgressTasks:    inProgress,
		CompletionRate:     completionRate,
		SessionsWithTodos:  len(todos),
		AvgTasksPerSession: float64(total) / float64(len(todos)),
	}
}

func analyzeFileChurn(fileHistory []claude.FileHistorySession) FileChurnAnalysis {
	if len(fileHistory) == 0 {
		return FileChurnAnalysis{}
	}

	var totalFiles, totalEdits, maxEdits int
	var totalBytes int64
	for _, fh := range fileHistory {
		totalFiles += fh.UniqueFiles
		totalEdits += fh.TotalEdits
		totalBytes += fh.TotalBytes
		if fh.TotalEdits > maxEdits {
			maxEdits = fh.TotalEdits
		}
	}

	var avgEditsPerFile float64
	if totalFiles > 0 {
		avgEditsPerFile = float64(totalEdits) / float64(totalFiles)
	}

	return FileChurnAnalysis{
		TotalSessions:      len(fileHistory),
		TotalFiles:         totalFiles,
		TotalEdits:         totalEdits,
		AvgEditsPerFile:    avgEditsPerFile,
		AvgFilesPerSession: float64(totalFiles) / float64(len(fileHistory)),
		MaxSessionEdits:    maxEdits,
		TotalBytes:         totalBytes,
	}
}
