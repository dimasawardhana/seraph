package db

import (
	"database/sql"
	"fmt"

	"seraph/internal/vocab"
)

// SchemaVersion is the version this build expects. Raise it whenever statements change,
// and append the new ones to upgrade; never edit a released one.
const SchemaVersion = 2

// createTasks is the v1 shape plus every column added since, so a brand-new database is
// correct in one pass and never has to be altered.
//
// The claim is identified by (claim_harness, claim_session) alone. An earlier draft also
// carried a per-claim token that release would have to present, but that token would have
// had to reach the harness somehow, and the only channel available is a snapshot that gets
// committed — where it would be a published secret. It is also redundant: once a claim is
// taken over, claim_session names the new holder, which already refuses the previous
// session's release.
//
// goal and acceptance carry no NOT NULL: SQLite cannot add a NOT NULL column without a
// default, so an existing v1 database could not be migrated. Both are required by the
// only write path, Create, which enforces them in Go.
var createTasks = []string{
	`CREATE TABLE IF NOT EXISTS tasks (
		id            TEXT PRIMARY KEY,
		title         TEXT    NOT NULL,
		goal          TEXT,
		description   TEXT,
		acceptance    TEXT,
		status        TEXT    NOT NULL DEFAULT '` + vocab.Default(vocab.Statuses) + `'
		              CHECK (status IN (` + vocab.Quoted(vocab.Statuses) + `)),
		triage        TEXT
		              CHECK (triage IS NULL OR triage IN (` + vocab.Quoted(vocab.Triages) + `)),
		priority      TEXT    NOT NULL DEFAULT '` + vocab.Default(vocab.Priorities) + `'
		              CHECK (priority IN (` + vocab.Quoted(vocab.Priorities) + `)),
		claim_harness TEXT,
		claim_session TEXT,
		claim_expires INTEGER,
		metadata      TEXT,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS id_sequence (
		id   INTEGER PRIMARY KEY CHECK (id = 1),
		last INTEGER NOT NULL
	)`,
	`INSERT OR IGNORE INTO id_sequence (id, last) VALUES (1, 100)`,
}

// upgrade holds statements that apply only to a database already carrying the v1 shape.
// A fresh database gets everything from createTasks, and running these against it fails
// with "duplicate column name" — which is how that was found.
var upgrade = []string{
	`ALTER TABLE tasks ADD COLUMN goal TEXT`,
	`ALTER TABLE tasks ADD COLUMN acceptance TEXT`,
	// v1 rows have no goal, and acceptance criteria did not exist then. Backfill rather
	// than fail: the history is worth keeping, and the Rules tell an agent to fill these
	// in when it next touches the task.
	`UPDATE tasks SET goal = title WHERE goal IS NULL OR goal = ''`,
}

func Migrate(handle *sql.DB) error {
	var current int
	if err := handle.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current >= SchemaVersion {
		return nil
	}

	tx, err := handle.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	statements := createTasks
	if current >= 1 {
		statements = upgrade
	}
	for i, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}
	// PRAGMA does not accept a bound parameter, and SchemaVersion is a constant.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return tx.Commit()
}

func AppliedVersion(handle *sql.DB) (int, error) {
	var version int
	if err := handle.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
