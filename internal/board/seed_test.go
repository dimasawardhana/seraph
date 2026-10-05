package board

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"seraph/internal/db"
	"seraph/internal/repo"
)

// newRepoWithSnapshot builds a real board through seraph's own migrations, with a committed
// board.json beside it — the exact shape a fresh clone arrives in.
func newRepoWithSnapshot(t *testing.T, snapshot string) (repo.Root, *sql.DB) {
	t.Helper()

	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if snapshot != "" {
		path := filepath.Join(root.StatePath(), SnapshotFile)
		if err := os.WriteFile(path, []byte(snapshot), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
	}
	return root, handle
}

const snapshotWithOneTask = `{
  "tasks": [
    {
      "id": "TASK-116",
      "title": "A fresh clone loses the board",
      "goal": "Restore it",
      "acceptance": "1. it works",
      "status": "backlog",
      "priority": "medium",
      "claim": null,
      "created_at": "2026-10-05T08:40:43Z",
      "updated_at": "2026-10-05T08:40:43Z"
    },
    {
      "id": "TASK-117",
      "title": "Carries a claim",
      "goal": "Keep the record",
      "status": "in_progress",
      "triage": "ready-for-agent",
      "priority": "high",
      "claim": {
        "harness": "omp",
        "session": "omp-session",
        "expires": "2026-10-05T09:40:43Z"
      },
      "created_at": "2026-10-05T08:41:00Z",
      "updated_at": "2026-10-05T08:41:00Z"
    }
  ]
}`

// The whole point: a clone arrives with the history in a file and an empty database, and the
// first thing that opens the board must put the history back.
func TestSeedRestoresAnEmptyBoardFromTheCommittedSnapshot(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, snapshotWithOneTask)

	seeded, err := SeedFromSnapshot(handle, root)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded != 2 {
		t.Errorf("expected 2 tasks restored, got %d", seeded)
	}

	tasks, err := Load(handle)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks on the board, got %d", len(tasks))
	}

	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}

	restored, ok := byID["TASK-116"]
	if !ok {
		t.Fatal("TASK-116 missing after seeding")
	}
	if restored.Title != "A fresh clone loses the board" || restored.Goal != "Restore it" {
		t.Errorf("text did not survive the round trip: %+v", restored)
	}
	if restored.Status != "backlog" || restored.Priority != "medium" {
		t.Errorf("status/priority did not survive: %+v", restored)
	}

	// A claim is a record as much as a lock. Dropping it would silently un-hold a task
	// somebody was actually working on.
	claimed, ok := byID["TASK-117"]
	if !ok {
		t.Fatal("TASK-117 missing after seeding")
	}
	if !claimed.Claimed() || claimed.ClaimSession != "omp-session" {
		t.Errorf("claim did not survive the seed: %+v", claimed)
	}
	if claimed.Triage != "ready-for-agent" {
		t.Errorf("triage did not survive: %q", claimed.Triage)
	}
}

// The guard the whole feature rests on. Seeding twice must not double the board, and must
// never replace work with an older snapshot.
func TestSeedNeverOverwritesAnExistingBoard(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, snapshotWithOneTask)

	if _, err := SeedFromSnapshot(handle, root); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if _, err := handle.Exec(
		`INSERT INTO tasks (id, title, status, priority, created_at, updated_at)
		 VALUES ('TASK-200', 'Live work', 'in_progress', 'high', 1, 1)`); err != nil {
		t.Fatalf("insert live task: %v", err)
	}

	seeded, err := SeedFromSnapshot(handle, root)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if seeded != 0 {
		t.Errorf("a board holding tasks must not be seeded, restored %d", seeded)
	}

	var n int
	if err := handle.QueryRow("SELECT count(*) FROM tasks").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 tasks and no duplicates, got %d", n)
	}
}

// Without advancing the sequence, the next create_task allocates 101 and collides with a task
// the snapshot already holds — a failure that would read as a corrupted board.
func TestSeedAdvancesTheIdSequencePastWhatItRestored(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, snapshotWithOneTask)

	if _, err := SeedFromSnapshot(handle, root); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var last int
	if err := handle.QueryRow(`SELECT last FROM id_sequence WHERE id = 1`).Scan(&last); err != nil {
		t.Fatalf("read id_sequence: %v", err)
	}
	if last < 117 {
		t.Errorf("id_sequence.last is %d, must be at least 117 or the next task collides", last)
	}
}

func TestSeedWithNoSnapshotIsNotAnError(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, "")

	seeded, err := SeedFromSnapshot(handle, root)
	if err != nil {
		t.Fatalf("a repository with no snapshot must seed cleanly: %v", err)
	}
	if seeded != 0 {
		t.Errorf("expected nothing restored, got %d", seeded)
	}
}

// A snapshot the current schema does not understand must fail whole. Half-applying one would
// leave a board that looks seeded and is not.
func TestSeedRejectsASnapshotTheSchemaDoesNotUnderstand(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, `{"tasks":[{"id":"TASK-1","title":"x","status":"invented","priority":"medium","created_at":"2026-10-05T08:40:43Z","updated_at":"2026-10-05T08:40:43Z"}]}`)

	if _, err := SeedFromSnapshot(handle, root); err == nil {
		t.Fatal("expected an unknown status to be refused rather than stored")
	}

	var n int
	if err := handle.QueryRow("SELECT count(*) FROM tasks").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("a refused seed must leave nothing behind, found %d tasks", n)
	}
}
