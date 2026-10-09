package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// v2Tasks is the schema as version 2 left it: everything except doc_refs. goal and
// acceptance stand last because the only boards that exist at v2 were built at v1 and
// upgraded, and the v1-to-v2 upgrade ALTERed those columns on at the end — the same order
// this repository's own live board carries. A v2 board built fresh with them mid-table has
// never existed.
const v2Tasks = `CREATE TABLE tasks (
	id            TEXT PRIMARY KEY,
	title         TEXT    NOT NULL,
	description   TEXT,
	status        TEXT    NOT NULL DEFAULT 'backlog',
	triage        TEXT,
	priority      TEXT    NOT NULL DEFAULT 'medium',
	claim_harness TEXT,
	claim_session TEXT,
	claim_expires INTEGER,
	metadata      TEXT,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL,
	goal          TEXT,
	acceptance    TEXT
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

// A database from a newer build must stop the line, not be read by a binary that does not
// know its shape. ADR-0004 refuses codegraph for precisely this silence.
func TestMigrateRefusesADatabaseFromANewerBuild(t *testing.T) {
	handle := openAtV2(t)
	if _, err := handle.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatalf("set version: %v", err)
	}

	err := Migrate(handle)
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("expected ErrNewerSchema, got %v", err)
	}
	for _, want := range []string{"v99", "v3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name both versions; %q missing from %q", want, err)
		}
	}

	// A refusal must not have touched the board on the way out.
	if version, _ := AppliedVersion(handle); version != 99 {
		t.Errorf("version changed to %d; a refused migration must leave it alone", version)
	}
}

// The trap this prevents: raising SchemaVersion without adding migrations[n] would append
// nothing for that step and still stamp the newer version as applied, on every board in the
// world. Checked as a property of the table rather than at runtime, so it fails where the
// forgetting happens instead of in the field.
func TestEveryVersionBelowCurrentHasAMigration(t *testing.T) {
	for version := 1; version < SchemaVersion; version++ {
		if len(migrations[version]) == 0 {
			t.Errorf("no migration from v%d to v%d: add migrations[%d], or Migrate will "+
				"record v%d without reaching it", version, version+1, version, SchemaVersion)
		}
	}
}

// The runtime half of the same trap: a version in the gap with no entry must be an error,
// not a silent skip that goes on to record the newer version. Shown by deleting the step the
// migrated board would need, then migrating.
func TestMigrateRefusesAGapItHasNoStatementFor(t *testing.T) {
	handle := openAtV2(t)

	saved := migrations[2]
	delete(migrations, 2)
	t.Cleanup(func() { migrations[2] = saved })

	err := Migrate(handle)
	if err == nil {
		t.Fatal("expected a missing migration from v2 to v3 to fail the run")
	}
	if !strings.Contains(err.Error(), "no migration from v2 to v3") {
		t.Errorf("error should name the missing step, got %q", err)
	}
	if version, _ := AppliedVersion(handle); version != 2 {
		t.Errorf("a gap recorded version %d; it must stay at 2", version)
	}
}

// A board built by the migration and one built from scratch must describe the same table:
// the same columns, in the same order, with the same types, defaults and constraints. Order
// is part of the promise — createTasks places goal, acceptance and doc_refs last, in the
// order the migrations ALTER them on — so a column added in the middle of createTasks fails
// here instead of drifting from every board the migration ever built.
func TestMigratedAndFreshBoardsAgreeOnTaskShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	fresh, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { fresh.Close() })
	if err := Migrate(fresh); err != nil {
		t.Fatalf("build a fresh board: %v", err)
	}

	migrated := openAtV2(t)
	if err := Migrate(migrated); err != nil {
		t.Fatalf("migrate a v2 board: %v", err)
	}

	freshShape := columnShape(t, fresh)
	migratedShape := columnShape(t, migrated)
	if len(freshShape) != len(migratedShape) {
		t.Fatalf("fresh board has %d columns, migrated one has %d", len(freshShape), len(migratedShape))
	}
	for i, want := range freshShape {
		if want.name != migratedShape[i].name {
			t.Fatalf("column %d is %q on a fresh board but %q on a migrated one",
				i, want.name, migratedShape[i].name)
		}
		if want.shape != migratedShape[i].shape {
			t.Errorf("column %q differs — fresh %q, migrated %q",
				want.name, want.shape, migratedShape[i].shape)
		}
	}
}

// columnShape reads each column's declared shape in declaration order — the order
// table_info reports, which is the order columns were added.
func columnShape(t *testing.T, handle *sql.DB) []column {
	t.Helper()

	rows, err := handle.Query("PRAGMA table_info(tasks)")
	if err != nil {
		t.Fatalf("read table_info: %v", err)
	}
	defer rows.Close()

	var shape []column
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		shape = append(shape, column{name,
			fmt.Sprintf("%s notnull=%d pk=%d default=%s", kind, notnull, pk, dflt.String)})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read table_info: %v", err)
	}
	return shape
}

type column struct{ name, shape string }

// A migration that fails part way must not record the version it did not reach. PRAGMA
// user_version rides the transaction, so the rollback takes it back too. This pins that: a
// later refactor running the PRAGMA outside the transaction would break it silently, and the
// board would then claim a shape it does not have.
func TestFailedMigrationLeavesTheVersionUntouched(t *testing.T) {
	handle := openAtV2(t)

	// Give the board the column the v2-to-v3 step adds, so that step fails on a duplicate.
	if _, err := handle.Exec("ALTER TABLE tasks ADD COLUMN doc_refs TEXT"); err != nil {
		t.Fatalf("pre-add column: %v", err)
	}

	if err := Migrate(handle); err == nil {
		t.Fatal("expected the duplicate column to fail the migration")
	}

	if version, _ := AppliedVersion(handle); version != 2 {
		t.Errorf("a failed migration recorded version %d; it must stay at 2", version)
	}
}
