package store

import (
	"database/sql"
	"time"
)

// CreateSnapshot inserts a new snapshot and returns its ID.
func (db *DB) CreateSnapshot(command, version string) (int64, error) {
	result, err := db.conn.Exec(
		"INSERT INTO snapshots (taken_at, command, version) VALUES (?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339), command, version,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetLatestSnapshot returns the most recent snapshot, or nil if none exist.
func (db *DB) GetLatestSnapshot() (*Snapshot, error) {
	row := db.conn.QueryRow("SELECT id, taken_at, command, version FROM snapshots ORDER BY id DESC LIMIT 1")
	return scanSnapshot(row)
}

// GetSnapshot returns a snapshot by ID.
func (db *DB) GetSnapshot(id int64) (*Snapshot, error) {
	row := db.conn.QueryRow("SELECT id, taken_at, command, version FROM snapshots WHERE id = ?", id)
	return scanSnapshot(row)
}

// GetSnapshotN returns the Nth most recent snapshot (1 = latest, 2 = previous, etc.).
func (db *DB) GetSnapshotN(n int) (*Snapshot, error) {
	row := db.conn.QueryRow(
		"SELECT id, taken_at, command, version FROM snapshots ORDER BY id DESC LIMIT 1 OFFSET ?",
		n-1,
	)
	return scanSnapshot(row)
}

func scanSnapshot(row *sql.Row) (*Snapshot, error) {
	var s Snapshot
	var takenAt string
	err := row.Scan(&s.ID, &takenAt, &s.Command, &s.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.TakenAt, _ = time.Parse(time.RFC3339, takenAt)
	return &s, nil
}

// InsertProjectScore inserts a project score for a snapshot.
func (db *DB) InsertProjectScore(ps *ProjectScore) error {
	_, err := db.conn.Exec(
		`INSERT INTO project_scores
		(snapshot_id, project, score, has_claude_md, has_dot_claude, has_local_settings,
		 session_count, last_session_date, primary_language, git_commit_30d)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ps.SnapshotID, ps.Project, ps.Score, ps.HasClaudeMD, ps.HasDotClaude,
		ps.HasLocalSettings, ps.SessionCount, ps.LastSessionDate, ps.PrimaryLanguage,
		ps.GitCommit30D,
	)
	return err
}

// InsertAggregateMetric inserts an aggregate metric for a snapshot.
func (db *DB) InsertAggregateMetric(snapshotID int64, name string, value float64, detail string) error {
	_, err := db.conn.Exec(
		"INSERT INTO aggregate_metrics (snapshot_id, metric_name, metric_value, detail) VALUES (?, ?, ?, ?)",
		snapshotID, name, value, detail,
	)
	return err
}

// InsertFrictionEvent inserts a friction event for a snapshot.
func (db *DB) InsertFrictionEvent(fe *FrictionEvent) error {
	_, err := db.conn.Exec(
		`INSERT INTO friction_events
		(snapshot_id, session_id, friction_type, count, detail, project, session_date)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		fe.SnapshotID, fe.SessionID, fe.FrictionType, fe.Count,
		fe.Detail, fe.Project, fe.SessionDate,
	)
	return err
}

// InsertSuggestion inserts a suggestion for a snapshot.
func (db *DB) InsertSuggestion(s *Suggestion) error {
	_, err := db.conn.Exec(
		`INSERT INTO suggestions
		(snapshot_id, category, priority, title, description, impact_score, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.SnapshotID, s.Category, s.Priority, s.Title, s.Description,
		s.ImpactScore, s.Status,
	)
	return err
}

// InsertAgentTask inserts an agent task record for a snapshot.
func (db *DB) InsertAgentTask(at *AgentTaskRow) error {
	_, err := db.conn.Exec(
		`INSERT INTO agent_tasks
		(snapshot_id, session_id, agent_id, agent_type, description, status,
		 duration_ms, total_tokens, tool_uses, background, needed_correction, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		at.SnapshotID, at.SessionID, at.AgentID, at.AgentType, at.Description,
		at.Status, at.DurationMs, at.TotalTokens, at.ToolUses, at.Background,
		at.NeededCorrection, at.CreatedAt,
	)
	return err
}

// InsertCustomMetric inserts a custom metric entry.
func (db *DB) InsertCustomMetric(cm *CustomMetricRow) error {
	_, err := db.conn.Exec(
		`INSERT INTO custom_metrics
		(logged_at, session_id, project, metric_name, metric_value, tags, note)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cm.LoggedAt, cm.SessionID, cm.Project, cm.MetricName,
		cm.MetricValue, cm.Tags, cm.Note,
	)
	return err
}

// GetAggregateMetrics returns all aggregate metrics for a snapshot.
func (db *DB) GetAggregateMetrics(snapshotID int64) ([]AggregateMetric, error) {
	rows, err := db.conn.Query(
		"SELECT id, snapshot_id, metric_name, metric_value, detail FROM aggregate_metrics WHERE snapshot_id = ?",
		snapshotID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var metrics []AggregateMetric
	for rows.Next() {
		var m AggregateMetric
		var detail sql.NullString
		if err := rows.Scan(&m.ID, &m.SnapshotID, &m.MetricName, &m.MetricValue, &detail); err != nil {
			return nil, err
		}
		m.Detail = detail.String
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

// GetProjectScores returns all project scores for a snapshot.
func (db *DB) GetProjectScores(snapshotID int64) ([]ProjectScore, error) {
	rows, err := db.conn.Query(
		`SELECT id, snapshot_id, project, score, has_claude_md, has_dot_claude,
		 has_local_settings, session_count, last_session_date, primary_language, git_commit_30d
		 FROM project_scores WHERE snapshot_id = ?`,
		snapshotID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var scores []ProjectScore
	for rows.Next() {
		var ps ProjectScore
		var lastDate, lang sql.NullString
		if err := rows.Scan(
			&ps.ID, &ps.SnapshotID, &ps.Project, &ps.Score,
			&ps.HasClaudeMD, &ps.HasDotClaude, &ps.HasLocalSettings,
			&ps.SessionCount, &lastDate, &lang, &ps.GitCommit30D,
		); err != nil {
			return nil, err
		}
		ps.LastSessionDate = lastDate.String
		ps.PrimaryLanguage = lang.String
		scores = append(scores, ps)
	}
	return scores, rows.Err()
}

// GetOpenSuggestions returns all suggestions with status "open".
func (db *DB) GetOpenSuggestions() ([]Suggestion, error) {
	rows, err := db.conn.Query(
		`SELECT id, snapshot_id, category, priority, title, description, impact_score, status
		 FROM suggestions WHERE status = 'open' ORDER BY impact_score DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		if err := rows.Scan(&s.ID, &s.SnapshotID, &s.Category, &s.Priority,
			&s.Title, &s.Description, &s.ImpactScore, &s.Status); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, rows.Err()
}

// ResolveSuggestion marks a suggestion as resolved.
func (db *DB) ResolveSuggestion(id int64) error {
	_, err := db.conn.Exec("UPDATE suggestions SET status = 'resolved' WHERE id = ?", id)
	return err
}
