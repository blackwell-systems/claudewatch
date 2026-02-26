package claude

// ParseAgentTasks extracts agent tasks from session transcript files stored in
// claudeDir/projects/*/*.jsonl. This replaces the previous approach of scanning
// ephemeral /tmp/claude-*/tasks/*.output files.
func ParseAgentTasks(claudeDir string) ([]AgentTask, error) {
	spans, err := ParseSessionTranscripts(claudeDir)
	if err != nil {
		return nil, err
	}

	tasks := make([]AgentTask, 0, len(spans))
	for _, span := range spans {
		status := "completed"
		if span.Killed {
			status = "killed"
		} else if !span.Success {
			status = "failed"
		}

		tasks = append(tasks, AgentTask{
			AgentID:     span.ToolUseID,
			AgentType:   span.AgentType,
			Description: span.Description,
			SessionID:   span.SessionID,
			Status:      status,
			DurationMs:  span.Duration.Milliseconds(),
			TotalTokens: 0, // Token counts not available in transcript data.
			ToolUses:    0, // Tool use counts not available in transcript data.
			Background:  span.Background,
			CreatedAt:   span.LaunchedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return tasks, nil
}
