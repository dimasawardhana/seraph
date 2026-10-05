package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// v2Tasks is the schema as it shipped at version 2: everything except doc_refs.
const v2Tasks = `CREATE TABLE tasks (
	id            TEXT PRIMARY KEY,
	title         TEXT    NOT NULL,
	goal          TEXT,
	description   TEXT,
	acceptance    TEXT,
	status        TEXT    NOT NULL DEFAULT 'backlog',
	triage        TEXT,
	priority      TEXT    NOT NULL DEFAULT 'medium',
	claim_harness TEXT,
	claim_session TEXT,
	claim_expires INTEGER,
	metadata      TEXT,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
)`

// openAtV2 builds a database shaped exactly as version 2 left it, with one task in it.
func openAtV2(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "state.db")
	handle, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if _, err := handle.Exec(v2Tasks); err != nil {
		t.Fatalf("create v2 schema: %v", err)
	}
	if _, err := handle.Exec(
		`INSERT INTO tasks (id, title, status, priority, created_at, updated_at)
		 VALUES ('TASK-7', 'Existing work', 'done', 'high', 1, 2)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := handle.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set version: %v", err)
	}
	return handle
}

func TestMigrateTakesAV2BoardToV3WithoutLosingAnything(t *testing.T) {
	handle := openAtV2(t)

	if err := Migrate(handle); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	version, err := AppliedVersion(handle)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
	}

	var title string
	if err := handle.QueryRow(`SELECT title FROM tasks WHERE id = 'TASK-7'`).Scan(&title); err != nil {
		t.Fatalf("the task must survive the migration: %v", err)
	}
	if title != "Existing work" {
		t.Errorf("task changed: %q", title)
	}

	// The column the migration exists to add.
	if _, err := handle.Exec(
		`INSERT INTO tasks (id, title, status, priority, doc_refs, created_at, updated_at)
		 VALUES ('TASK-8', 'New work', 'backlog', 'medium', '["docs/a.md"]', 1, 1)`); err != nil {
		t.Errorf("doc_refs missing after migration: %v", err)
	}
}

// The trap this replaced: one shared upgrade list meant a v2 board re-running the v1
// ALTERs, which fails on a duplicate column and leaves the repository unable to open.
func TestMigrateDoesNotReRunAnOlderMigration(t *testing.T) {
	handle := openAtV2(t)

	for i := range 2 {
		if err := Migrate(handle); err != nil {
			t.Fatalf("migrate pass %d: %v", i+1, err)
		}
	}

}

func TestMigrateIsANoOpOnACurrentBoard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	handle, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if err := Migrate(handle); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(handle); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
