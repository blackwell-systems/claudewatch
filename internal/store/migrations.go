package store

import "fmt"

// currentSchemaVersion is the latest schema version.
const currentSchemaVersion = 1

// Migrate runs forward migrations to bring the database schema up to date.
func (db *DB) Migrate() error {
	// Create the schema_version table if it does not exist.
	if _, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	version := 0
	row := db.conn.QueryRow("SELECT version FROM schema_version LIMIT 1")
	if err := row.Scan(&version); err != nil {
		// No rows means version 0 (fresh database).
		version = 0
	}

	if version < 1 {
		if err := db.migrateV1(); err != nil {
			return fmt.Errorf("migration v1: %w", err)
		}
	}

	return nil
}

// migrateV1 creates all initial tables and indexes.
func (db *DB) migrateV1() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS snapshots (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			taken_at    TEXT NOT NULL,
			command     TEXT NOT NULL,
			version     TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS project_scores (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id        INTEGER NOT NULL REFERENCES snapshots(id),
			project            TEXT NOT NULL,
			score              REAL NOT NULL,
			has_claude_md      BOOLEAN NOT NULL,
			has_dot_claude     BOOLEAN NOT NULL,
			has_local_settings BOOLEAN NOT NULL,
			session_count      INTEGER NOT NULL,
			last_session_date  TEXT,
			primary_language   TEXT,
			git_commit_30d     INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS aggregate_metrics (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id  INTEGER NOT NULL REFERENCES snapshots(id),
			metric_name  TEXT NOT NULL,
			metric_value REAL NOT NULL,
			detail       TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS friction_events (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id   INTEGER NOT NULL REFERENCES snapshots(id),
			session_id    TEXT NOT NULL,
			friction_type TEXT NOT NULL,
			count         INTEGER NOT NULL,
			detail        TEXT,
			project       TEXT,
			session_date  TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS suggestions (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id  INTEGER NOT NULL REFERENCES snapshots(id),
			category     TEXT NOT NULL,
			priority     INTEGER NOT NULL,
			title        TEXT NOT NULL,
			description  TEXT NOT NULL,
			impact_score REAL NOT NULL,
			status       TEXT NOT NULL DEFAULT 'open'
		)`,

		`CREATE TABLE IF NOT EXISTS agent_tasks (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id       INTEGER NOT NULL REFERENCES snapshots(id),
			session_id        TEXT NOT NULL,
			agent_id          TEXT NOT NULL,
			agent_type        TEXT NOT NULL,
			description       TEXT,
			status            TEXT NOT NULL,
			duration_ms       INTEGER,
			total_tokens      INTEGER,
			tool_uses         INTEGER,
			background        BOOLEAN DEFAULT false,
			needed_correction BOOLEAN DEFAULT false,
			created_at        TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS custom_metrics (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			logged_at    TEXT NOT NULL,
			session_id   TEXT,
			project      TEXT,
			metric_name  TEXT NOT NULL,
			metric_value REAL,
			tags         TEXT,
			note         TEXT
		)`,

		// Indexes.
		`CREATE INDEX IF NOT EXISTS idx_project_scores_snapshot ON project_scores(snapshot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_aggregate_snapshot ON aggregate_metrics(snapshot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_friction_snapshot ON friction_events(snapshot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_suggestions_status ON suggestions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_tasks_snapshot ON agent_tasks(snapshot_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_tasks_type ON agent_tasks(agent_type)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_metrics_name ON custom_metrics(metric_name)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_metrics_session ON custom_metrics(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_metrics_project ON custom_metrics(project)`,
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:40], err)
		}
	}

	// Set schema version.
	if _, err := tx.Exec("DELETE FROM schema_version"); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", currentSchemaVersion); err != nil {
		return err
	}

	return tx.Commit()
}
