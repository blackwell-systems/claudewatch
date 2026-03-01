package claude

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseSAWTag parses a SAW coordination tag from a task description.
// Expected format: "[SAW:wave{N}:agent-{X}] rest of description"
// Returns wave number (≥1), agent letter (e.g., "A"), and true if valid.
// Returns 0, "", false if no SAW tag is present or the format is malformed.
func ParseSAWTag(description string) (wave int, agent string, ok bool) {
	// Must start with "[SAW:wave"
	if !strings.HasPrefix(description, "[SAW:wave") {
		return 0, "", false
	}

	// Find the closing bracket
	closingIdx := strings.Index(description, "]")
	if closingIdx < 0 {
		return 0, "", false
	}

	// Extract the tag content between [ and ]
	tagContent := description[1:closingIdx] // strip leading "[" and trailing "]"

	// Split on ":" — expect exactly 3 parts: "SAW", "wave{N}", "agent-{X}"
	parts := strings.Split(tagContent, ":")
	if len(parts) != 3 {
		return 0, "", false
	}

	if parts[0] != "SAW" {
		return 0, "", false
	}

	// Parse wave number: strip "wave" prefix
	wavePart := parts[1]
	if !strings.HasPrefix(wavePart, "wave") {
		return 0, "", false
	}
	waveNumStr := wavePart[len("wave"):]
	waveNum, err := strconv.Atoi(waveNumStr)
	if err != nil || waveNum <= 0 {
		return 0, "", false
	}

	// Parse agent: strip "agent-" prefix
	agentPart := parts[2]
	if !strings.HasPrefix(agentPart, "agent-") {
		return 0, "", false
	}
	agentStr := agentPart[len("agent-"):]
	if agentStr == "" {
		return 0, "", false
	}

	return waveNum, agentStr, true
}

// SAWAgentRun represents a single agent's execution within a SAW wave.
type SAWAgentRun struct {
	Agent       string    // e.g., "A", "B", "C"
	Description string    // full description from the Task call (includes tag)
	Status      string    // "completed", "failed", "killed"
	DurationMs  int64
	LaunchedAt  time.Time
	CompletedAt time.Time
}

// SAWWave represents one wave of parallel SAW agents within a session.
type SAWWave struct {
	Wave       int           // wave number from the tag
	Agents     []SAWAgentRun // sorted by Agent letter
	StartedAt  time.Time     // earliest LaunchedAt across all agents in this wave
	EndedAt    time.Time     // latest CompletedAt across all agents in this wave
	DurationMs int64         // EndedAt - StartedAt wall-clock milliseconds
}

// SAWSession represents a complete SAW execution within one Claude Code session.
type SAWSession struct {
	SessionID   string    // from AgentSpan.SessionID
	ProjectHash string    // from AgentSpan.ProjectHash
	Waves       []SAWWave // sorted by wave number
	TotalAgents int       // sum of agents across all waves
}

// ComputeSAWWaves scans spans for SAW-tagged agents and groups them into SAWSession
// structures. Spans without a valid ParseSAWTag result are ignored.
// Sessions are returned in no guaranteed order. Waves within each session are
// sorted by wave number. Agents within each wave are sorted by Agent letter.
// Returns an empty slice (not nil) if no SAW-tagged spans are found.
func ComputeSAWWaves(spans []AgentSpan) []SAWSession {
	// Track session metadata
	sessionMeta := make(map[string]string) // sessionID -> projectHash

	// sessionID -> waveN -> []SAWAgentRun
	accumulator := make(map[string]map[int][]SAWAgentRun)

	for _, span := range spans {
		waveNum, agentStr, ok := ParseSAWTag(span.Description)
		if !ok {
			continue
		}

		// Determine status
		var status string
		if span.Killed {
			status = "killed"
		} else if !span.Success {
			status = "failed"
		} else {
			status = "completed"
		}

		durationMs := span.CompletedAt.Sub(span.LaunchedAt).Milliseconds()

		run := SAWAgentRun{
			Agent:       agentStr,
			Description: span.Description,
			Status:      status,
			DurationMs:  durationMs,
			LaunchedAt:  span.LaunchedAt,
			CompletedAt: span.CompletedAt,
		}

		if _, exists := accumulator[span.SessionID]; !exists {
			accumulator[span.SessionID] = make(map[int][]SAWAgentRun)
		}
		accumulator[span.SessionID][waveNum] = append(accumulator[span.SessionID][waveNum], run)
		sessionMeta[span.SessionID] = span.ProjectHash
	}

	if len(accumulator) == 0 {
		return []SAWSession{}
	}

	// Build sorted SAWSessions
	sessions := make([]SAWSession, 0, len(accumulator))

	for sessionID, waveMap := range accumulator {
		// Build sorted waves
		waveNums := make([]int, 0, len(waveMap))
		for wn := range waveMap {
			waveNums = append(waveNums, wn)
		}
		sort.Ints(waveNums)

		waves := make([]SAWWave, 0, len(waveNums))
		totalAgents := 0

		for _, wn := range waveNums {
			runs := waveMap[wn]

			// Sort agents by Agent letter
			sort.Slice(runs, func(i, j int) bool {
				return runs[i].Agent < runs[j].Agent
			})

			// Compute StartedAt (earliest LaunchedAt) and EndedAt (latest CompletedAt)
			var startedAt, endedAt time.Time
			for _, r := range runs {
				if startedAt.IsZero() || r.LaunchedAt.Before(startedAt) {
					startedAt = r.LaunchedAt
				}
				if endedAt.IsZero() || r.CompletedAt.After(endedAt) {
					endedAt = r.CompletedAt
				}
			}

			durationMs := endedAt.Sub(startedAt).Milliseconds()

			waves = append(waves, SAWWave{
				Wave:       wn,
				Agents:     runs,
				StartedAt:  startedAt,
				EndedAt:    endedAt,
				DurationMs: durationMs,
			})

			totalAgents += len(runs)
		}

		sessions = append(sessions, SAWSession{
			SessionID:   sessionID,
			ProjectHash: sessionMeta[sessionID],
			Waves:       waves,
			TotalAgents: totalAgents,
		})
	}

	// Sort sessions by SessionID for deterministic output
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].SessionID < sessions[j].SessionID
	})

	return sessions
}
