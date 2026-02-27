package analyzer

import (
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// SessionIntent classifies what a session was primarily doing.
type SessionIntent string

const (
	IntentExploration    SessionIntent = "exploration"    // >60% read tools
	IntentImplementation SessionIntent = "implementation" // >60% write tools
	IntentMixed          SessionIntent = "mixed"          // neither dominates
)

// readTools are tools that indicate exploration/understanding.
var readTools = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "WebSearch": true, "WebFetch": true,
}

// writeTools are tools that indicate implementation/production.
var writeTools = map[string]bool{
	"Edit": true, "Write": true, "NotebookEdit": true,
}

// SessionConfidence holds per-session intent classification and outcome data.
type SessionConfidence struct {
	SessionID   string        `json:"session_id"`
	ProjectPath string        `json:"project_path"`
	Intent      SessionIntent `json:"intent"`
	ReadRatio   float64       `json:"read_ratio"`  // read tools / total tools
	WriteRatio  float64       `json:"write_ratio"` // write tools / total tools
	Commits     int           `json:"commits"`
	TotalTools  int           `json:"total_tools"`
}

// ProjectConfidence holds the aggregate confidence signal for a single project.
type ProjectConfidence struct {
	ProjectName   string  `json:"project_name"`
	ProjectPath   string  `json:"project_path"`
	Sessions      int     `json:"sessions"`
	AvgReadRatio  float64 `json:"avg_read_ratio"`
	AvgWriteRatio float64 `json:"avg_write_ratio"`

	// ExplorationRate is the fraction of sessions classified as exploration.
	ExplorationRate float64 `json:"exploration_rate"`

	// ImplementationRate is the fraction classified as implementation.
	ImplementationRate float64 `json:"implementation_rate"`

	// ExplorationCommitRate is the avg commits in exploration sessions.
	// Low value + high exploration rate = Claude doesn't know the project.
	ExplorationCommitRate float64 `json:"exploration_commit_rate"`

	// ImplementationCommitRate is the avg commits in implementation sessions.
	ImplementationCommitRate float64 `json:"implementation_commit_rate"`

	// ConfidenceScore is 0-100 representing how confidently Claude can act
	// in this project. High write ratio + high commits = high confidence.
	// High read ratio + low commits = low confidence.
	ConfidenceScore float64 `json:"confidence_score"`

	// Signal is a human-readable assessment.
	Signal string `json:"signal"`
}

// ConfidenceAnalysis is the top-level result.
type ConfidenceAnalysis struct {
	// Projects sorted by confidence score ascending (lowest confidence first).
	Projects []ProjectConfidence `json:"projects"`

	// GlobalAvgReadRatio is the average read ratio across all sessions.
	GlobalAvgReadRatio float64 `json:"global_avg_read_ratio"`

	// GlobalAvgWriteRatio is the average write ratio across all sessions.
	GlobalAvgWriteRatio float64 `json:"global_avg_write_ratio"`

	// LowConfidenceCount is projects scoring below 40.
	LowConfidenceCount int `json:"low_confidence_count"`
}

// classifySession determines the intent of a single session from its tool counts.
func classifySession(s claude.SessionMeta) SessionConfidence {
	var readCount, writeCount, total int
	for tool, count := range s.ToolCounts {
		total += count
		if readTools[tool] {
			readCount += count
		}
		if writeTools[tool] {
			writeCount += count
		}
	}

	sc := SessionConfidence{
		SessionID:   s.SessionID,
		ProjectPath: s.ProjectPath,
		Commits:     s.GitCommits,
		TotalTools:  total,
	}

	if total > 0 {
		sc.ReadRatio = float64(readCount) / float64(total)
		sc.WriteRatio = float64(writeCount) / float64(total)
	}

	switch {
	case sc.ReadRatio > 0.6:
		sc.Intent = IntentExploration
	case sc.WriteRatio > 0.6:
		sc.Intent = IntentImplementation
	default:
		sc.Intent = IntentMixed
	}

	return sc
}

// AnalyzeConfidence computes per-project confidence scores from session tool ratios
// and commit data. A project where Claude spends most of its time reading with few
// commits is one where the CLAUDE.md likely lacks enough context for confident action.
func AnalyzeConfidence(sessions []claude.SessionMeta) ConfidenceAnalysis {
	if len(sessions) == 0 {
		return ConfidenceAnalysis{}
	}

	// Classify each session.
	classified := make([]SessionConfidence, 0, len(sessions))
	var globalReadSum, globalWriteSum float64

	for _, s := range sessions {
		sc := classifySession(s)
		if sc.TotalTools == 0 {
			continue // skip sessions with no tool usage
		}
		classified = append(classified, sc)
		globalReadSum += sc.ReadRatio
		globalWriteSum += sc.WriteRatio
	}

	if len(classified) == 0 {
		return ConfidenceAnalysis{}
	}

	n := float64(len(classified))

	// Group by project.
	byProject := make(map[string][]SessionConfidence)
	for _, sc := range classified {
		if sc.ProjectPath == "" {
			continue
		}
		byProject[sc.ProjectPath] = append(byProject[sc.ProjectPath], sc)
	}

	var projects []ProjectConfidence
	for path, projSessions := range byProject {
		if len(projSessions) < 2 {
			continue // need multiple sessions for a meaningful signal
		}

		pc := buildProjectConfidence(path, projSessions)
		projects = append(projects, pc)
	}

	// Sort by confidence score ascending (worst first).
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ConfidenceScore < projects[j].ConfidenceScore
	})

	lowCount := 0
	for _, p := range projects {
		if p.ConfidenceScore < 40 {
			lowCount++
		}
	}

	return ConfidenceAnalysis{
		Projects:            projects,
		GlobalAvgReadRatio:  globalReadSum / n,
		GlobalAvgWriteRatio: globalWriteSum / n,
		LowConfidenceCount:  lowCount,
	}
}

func buildProjectConfidence(path string, sessions []SessionConfidence) ProjectConfidence {
	pc := ProjectConfidence{
		ProjectName: filepath.Base(path),
		ProjectPath: path,
		Sessions:    len(sessions),
	}

	var readSum, writeSum float64
	var explorationCount, implCount int
	var explorationCommits, implCommits int

	for _, sc := range sessions {
		readSum += sc.ReadRatio
		writeSum += sc.WriteRatio

		switch sc.Intent {
		case IntentExploration:
			explorationCount++
			explorationCommits += sc.Commits
		case IntentImplementation:
			implCount++
			implCommits += sc.Commits
		}
	}

	n := float64(len(sessions))
	pc.AvgReadRatio = readSum / n
	pc.AvgWriteRatio = writeSum / n
	pc.ExplorationRate = float64(explorationCount) / n
	pc.ImplementationRate = float64(implCount) / n

	if explorationCount > 0 {
		pc.ExplorationCommitRate = float64(explorationCommits) / float64(explorationCount)
	}
	if implCount > 0 {
		pc.ImplementationCommitRate = float64(implCommits) / float64(implCount)
	}

	// Confidence score: 0-100.
	//
	// High confidence = high write ratio + high commits per session.
	// Low confidence = high read ratio + low commits per session.
	//
	// Components:
	//   writeRatioScore (0-40): how much of the work is implementation
	//   commitDensity (0-40): commits per session normalized
	//   explorationPenalty (0-20): penalty for high exploration with low commits

	// Write ratio score: 0 at 0% write, 40 at 60%+ write.
	writeRatioScore := pc.AvgWriteRatio / 0.6 * 40
	if writeRatioScore > 40 {
		writeRatioScore = 40
	}

	// Commit density: avg commits across all sessions. 0 at 0, 40 at 3+.
	totalCommits := 0
	for _, sc := range sessions {
		totalCommits += sc.Commits
	}
	avgCommits := float64(totalCommits) / n
	commitDensity := avgCommits / 3.0 * 40
	if commitDensity > 40 {
		commitDensity = 40
	}

	// Exploration penalty: if >50% exploration sessions AND those sessions
	// average <1 commit, penalty up to 20 points.
	explorationPenalty := 0.0
	if pc.ExplorationRate > 0.5 && pc.ExplorationCommitRate < 1.0 {
		// Scale: 0 penalty at 50% exploration, 20 at 100%.
		ratePenalty := (pc.ExplorationRate - 0.5) / 0.5 * 10
		// Scale: 0 penalty at 1 commit/exploration, 10 at 0 commits.
		commitPenalty := (1.0 - pc.ExplorationCommitRate) * 10
		explorationPenalty = ratePenalty + commitPenalty
		if explorationPenalty > 20 {
			explorationPenalty = 20
		}
	}

	pc.ConfidenceScore = writeRatioScore + commitDensity - explorationPenalty
	if pc.ConfidenceScore < 0 {
		pc.ConfidenceScore = 0
	}
	if pc.ConfidenceScore > 100 {
		pc.ConfidenceScore = 100
	}

	// Signal.
	switch {
	case pc.ConfidenceScore >= 70:
		pc.Signal = "high confidence — Claude acts decisively in this project"
	case pc.ConfidenceScore >= 40:
		pc.Signal = "moderate — some exploration needed before acting"
	default:
		pc.Signal = "low confidence — Claude spends most time reading, CLAUDE.md may need more context"
	}

	return pc
}
