package store

import (
	"database/sql"
	"fmt"
)

// UpsertProjectBaseline stores or replaces the statistical baseline for a project.
func (db *DB) UpsertProjectBaseline(b ProjectBaseline) error {
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO project_baselines
			(project, computed_at, session_count, avg_cost_usd, stddev_cost_usd,
			 avg_friction, stddev_friction, avg_commits, saw_session_frac)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		b.Project,
		b.ComputedAt,
		b.SessionCount,
		b.AvgCostUSD,
		b.StddevCostUSD,
		b.AvgFriction,
		b.StddevFriction,
		b.AvgCommits,
		b.SAWSessionFrac,
	)
	if err != nil {
		return fmt.Errorf("upserting project baseline for %q: %w", b.Project, err)
	}
	return nil
}

// GetProjectBaseline retrieves the stored baseline for a project.
// Returns nil, nil if no baseline exists yet.
func (db *DB) GetProjectBaseline(project string) (*ProjectBaseline, error) {
	row := db.conn.QueryRow(`
		SELECT project, computed_at, session_count, avg_cost_usd, stddev_cost_usd,
		       avg_friction, stddev_friction, avg_commits, saw_session_frac
		FROM project_baselines
		WHERE project = ?
	`, project)

	var b ProjectBaseline
	err := row.Scan(
		&b.Project,
		&b.ComputedAt,
		&b.SessionCount,
		&b.AvgCostUSD,
		&b.StddevCostUSD,
		&b.AvgFriction,
		&b.StddevFriction,
		&b.AvgCommits,
		&b.SAWSessionFrac,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting project baseline for %q: %w", project, err)
	}
	return &b, nil
}

// ListProjectBaselines returns all stored baselines, sorted by project name.
func (db *DB) ListProjectBaselines() ([]ProjectBaseline, error) {
	rows, err := db.conn.Query(`
		SELECT project, computed_at, session_count, avg_cost_usd, stddev_cost_usd,
		       avg_friction, stddev_friction, avg_commits, saw_session_frac
		FROM project_baselines
		ORDER BY project ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing project baselines: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var baselines []ProjectBaseline
	for rows.Next() {
		var b ProjectBaseline
		if err := rows.Scan(
			&b.Project,
			&b.ComputedAt,
			&b.SessionCount,
			&b.AvgCostUSD,
			&b.StddevCostUSD,
			&b.AvgFriction,
			&b.StddevFriction,
			&b.AvgCommits,
			&b.SAWSessionFrac,
		); err != nil {
			return nil, fmt.Errorf("scanning project baseline: %w", err)
		}
		baselines = append(baselines, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating project baselines: %w", err)
	}

	return baselines, nil
}
