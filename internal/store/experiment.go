package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Experiment represents an A/B experiment for a project.
type Experiment struct {
	ID        int64      `json:"id"`
	Project   string     `json:"project"`
	StartedAt time.Time  `json:"started_at"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	Active    bool       `json:"active"`
	Note      string     `json:"note,omitempty"`
}

// ExperimentSession records which variant a session was assigned to.
type ExperimentSession struct {
	ExperimentID int64  `json:"experiment_id"`
	SessionID    string `json:"session_id"`
	Variant      string `json:"variant"` // "a" or "b"
}

// CreateExperiment creates a new active experiment for the given project.
// Returns an error if there is already an active experiment for the project.
func (db *DB) CreateExperiment(project, note string) (int64, error) {
	existing, err := db.GetActiveExperiment(project)
	if err != nil {
		return 0, fmt.Errorf("checking for active experiment: %w", err)
	}
	if existing != nil {
		return 0, fmt.Errorf("active experiment already exists for project %q — stop it first", project)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.conn.Exec(
		`INSERT INTO experiments (project, started_at, active, note) VALUES (?, ?, true, ?)`,
		project, now, note,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting experiment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}
	return id, nil
}

// GetActiveExperiment returns the active experiment for the given project,
// or nil if no active experiment exists.
func (db *DB) GetActiveExperiment(project string) (*Experiment, error) {
	row := db.conn.QueryRow(
		`SELECT id, project, started_at, stopped_at, active, note
		 FROM experiments
		 WHERE project = ? AND active = 1
		 ORDER BY started_at DESC
		 LIMIT 1`,
		project,
	)

	exp, err := scanExperiment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return exp, nil
}

// StopExperiment marks the experiment with the given ID as inactive and
// sets its stopped_at timestamp to the current time.
func (db *DB) StopExperiment(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.conn.Exec(
		`UPDATE experiments SET active = false, stopped_at = ? WHERE id = ?`,
		now, id,
	)
	return err
}

// RecordSessionVariant records or replaces the variant assignment for a
// session within an experiment.
func (db *DB) RecordSessionVariant(experimentID int64, sessionID, variant string) error {
	_, err := db.conn.Exec(
		`INSERT OR REPLACE INTO experiment_sessions (experiment_id, session_id, variant)
		 VALUES (?, ?, ?)`,
		experimentID, sessionID, variant,
	)
	return err
}

// GetExperimentSessions returns all session assignments for the given experiment.
func (db *DB) GetExperimentSessions(experimentID int64) ([]ExperimentSession, error) {
	rows, err := db.conn.Query(
		`SELECT experiment_id, session_id, variant
		 FROM experiment_sessions
		 WHERE experiment_id = ?`,
		experimentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []ExperimentSession
	for rows.Next() {
		var es ExperimentSession
		if err := rows.Scan(&es.ExperimentID, &es.SessionID, &es.Variant); err != nil {
			return nil, err
		}
		sessions = append(sessions, es)
	}
	return sessions, rows.Err()
}

// ListExperiments returns all experiments for the given project, ordered by
// started_at descending.
func (db *DB) ListExperiments(project string) ([]Experiment, error) {
	rows, err := db.conn.Query(
		`SELECT id, project, started_at, stopped_at, active, note
		 FROM experiments
		 WHERE project = ?
		 ORDER BY started_at DESC`,
		project,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var experiments []Experiment
	for rows.Next() {
		exp, err := scanExperimentRow(rows)
		if err != nil {
			return nil, err
		}
		experiments = append(experiments, *exp)
	}
	return experiments, rows.Err()
}

// scanExperiment scans a single *sql.Row into an Experiment.
func scanExperiment(row *sql.Row) (*Experiment, error) {
	var exp Experiment
	var startedAtStr string
	var stoppedAtStr *string

	if err := row.Scan(&exp.ID, &exp.Project, &startedAtStr, &stoppedAtStr, &exp.Active, &exp.Note); err != nil {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing started_at %q: %w", startedAtStr, err)
	}
	exp.StartedAt = t

	if stoppedAtStr != nil {
		ts, err := time.Parse(time.RFC3339, *stoppedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing stopped_at %q: %w", *stoppedAtStr, err)
		}
		exp.StoppedAt = &ts
	}

	return &exp, nil
}

// scanExperimentRow scans a *sql.Rows row into an Experiment.
func scanExperimentRow(rows *sql.Rows) (*Experiment, error) {
	var exp Experiment
	var startedAtStr string
	var stoppedAtStr *string

	if err := rows.Scan(&exp.ID, &exp.Project, &startedAtStr, &stoppedAtStr, &exp.Active, &exp.Note); err != nil {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing started_at %q: %w", startedAtStr, err)
	}
	exp.StartedAt = t

	if stoppedAtStr != nil {
		ts, err := time.Parse(time.RFC3339, *stoppedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing stopped_at %q: %w", *stoppedAtStr, err)
		}
		exp.StoppedAt = &ts
	}

	return &exp, nil
}
