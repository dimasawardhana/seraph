package board

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"seraph/internal/repo"
)

// SnapshotFile is the committed JSON board beside the database.
const SnapshotFile = "board.json"

// SeedFromSnapshot populates an empty board from the committed board.json next to it, and
// reports how many tasks it restored.
//
// .seraph/.gitignore keeps state.db out of the repository — a database is not a document —
// while KANBAN.md and board.json are committed, because they are meant to be read. A fresh
// clone therefore arrives holding the project's whole history in two files and no database,
// and the first tool call on it renders an empty board over the top of all of that. This is
// what stops that happening.
//
// It refuses to touch a board that already holds a single task, so it can never replace live
// work with an older snapshot. A board with claims on it is not empty, and is left alone.
//
// Claims are restored along with the tasks, expired or not. A claim from a commit is almost
// always stale by the time anyone clones, and the gate already treats an expired claim as no
// claim — but dropping the record would quietly un-hold a task somebody had actually been
// working on, which is the one outcome worse than a stale claim.
func SeedFromSnapshot(handle *sql.DB, r repo.Root) (int, error) {
	var existing int
	if err := handle.QueryRow("SELECT count(*) FROM tasks").Scan(&existing); err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}
	if existing > 0 {
		return 0, nil
	}

	path := filepath.Join(r.StatePath(), SnapshotFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		// No snapshot is the ordinary case: a repository that has never been committed
		// has nothing to restore, and that is not an error.
		return 0, nil
	}

	var snap boardJSON
	if err := json.Unmarshal(raw, &snap); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(snap.Tasks) == 0 {
		return 0, nil
	}

	tx, err := handle.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin seed: %w", err)
	}
	defer tx.Rollback()

	highest := 0
	for _, t := range snap.Tasks {
		created, err := unstamp(t.CreatedAt)
		if err != nil {
			return 0, fmt.Errorf("task %s: created_at %q: %w", t.ID, t.CreatedAt, err)
		}
		updated, err := unstamp(t.UpdatedAt)
		if err != nil {
			return 0, fmt.Errorf("task %s: updated_at %q: %w", t.ID, t.UpdatedAt, err)
		}

		var harness, session any
		var expires any
		if t.Claim != nil {
			harness, session = t.Claim.Harness, t.Claim.Session
			if at, err := unstamp(t.Claim.Expires); err == nil {
				expires = at
			}
		}
		var triage any
		if t.Triage != "" {
			triage = t.Triage
		}

		// A status or priority the current schema does not know fails the CHECK here
		// rather than being quietly coerced, and takes the whole seed with it. A
		// snapshot from a future version must not half-apply.
		if _, err := tx.Exec(
			`INSERT INTO tasks (id, title, goal, description, acceptance, status, triage,
			                    priority, doc_refs, claim_harness, claim_session, claim_expires, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.Title, t.Goal, t.Description, t.Acceptance, t.Status, triage, t.Priority,
			encodeReferences(t.References), harness, session, expires, created, updated); err != nil {
			return 0, fmt.Errorf("seed %s: %w", t.ID, err)
		}

		if n, ok := taskNumber(t.ID); ok && n > highest {
			highest = n
		}
	}

	// Without this the next create_task allocates from the seeded default of 100 and
	// collides with a task the snapshot already contains. That failure would look like a
	// corrupted board rather than the arithmetic it is.
	if highest > 0 {
		if _, err := tx.Exec(`UPDATE id_sequence SET last = ? WHERE id = 1 AND last < ?`, highest, highest); err != nil {
			return 0, fmt.Errorf("advance id_sequence: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit seed: %w", err)
	}
	return len(snap.Tasks), nil
}

// unstamp parses the RFC3339 timestamps the renderer writes.
func unstamp(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// taskNumber reads TASK-101 as 101, and reports false for an id it cannot reason about
// rather than guessing — a board seeded from a snapshot with odd ids keeps its default
// sequence instead of being advanced to a number nothing supports.
func taskNumber(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, "TASK-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil
}
