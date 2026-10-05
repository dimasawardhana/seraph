package doctor

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"seraph/internal/board"
	"seraph/internal/db"
	"seraph/internal/repo"
)

func Run(_ context.Context, w io.Writer) error {
	root, err := repo.Resolve(".")
	if err != nil {
		return err
	}

	marker := root.Marker
	if !root.Found() {
		marker = "none found — assumed from the working directory"
	}
	field(w, "repository", root.Path)
	field(w, "marker", marker)

	handle, err := db.Open(root)
	if err != nil {
		field(w, "database", "UNREACHABLE — "+err.Error())
		return errors.New("database unreachable")
	}
	defer handle.Close()

	// Doctor reports the seed rather than refusing to run. A snapshot this build cannot
	// read is worth saying out loud, but doctor exists to describe a board, and the fix
	// is the reader's — the servers are where a bad snapshot must stop the line.
	seeded, seedErr := board.SeedFromSnapshot(handle, root)
	switch {
	case seedErr != nil:
		field(w, "board", "NOT RESTORED from "+board.SnapshotFile+" — "+seedErr.Error())
	case seeded > 0:
		field(w, "board", fmt.Sprintf("restored %d task(s) from the committed %s", seeded, board.SnapshotFile))
	}
	field(w, "database", root.DBPath())

	journal, err := scalar(handle, "PRAGMA journal_mode")
	if err != nil {
		return err
	}
	field(w, "journal", journal)

	applied, err := db.AppliedVersion(handle)
	if err != nil {
		return err
	}
	schema := fmt.Sprintf("v%d (this build expects v%d)", applied, db.SchemaVersion)
	if applied != db.SchemaVersion {
		schema += " — MISMATCH"
	}
	field(w, "schema", schema)

	var tasks int
	if err := handle.QueryRow("SELECT count(*) FROM tasks").Scan(&tasks); err != nil {
		return fmt.Errorf("count tasks: %w", err)
	}
	field(w, "tasks", fmt.Sprintf("%d", tasks))

	for _, name := range []string{board.KanbanFile, board.JSONFile} {
		field(w, name, snapshotState(handle, root, name))
	}

	reportHarnesses(w, root)

	if applied != db.SchemaVersion {
		return errors.New("schema version mismatch")
	}
	return nil
}

func snapshotState(handle *sql.DB, r repo.Root, name string) string {
	path := filepath.Join(r.StatePath(), name)
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return "absent — any tool call writes it"
	}

	tasks, err := board.Load(handle)
	if err != nil {
		return "unreadable — " + err.Error()
	}
	snapshot := board.New(tasks)

	var current []byte
	if name == board.KanbanFile {
		current = snapshot.Markdown()
	} else if current, err = snapshot.JSON(); err != nil {
		return "unrenderable — " + err.Error()
	}

	age := time.Since(onDiskModTime(path)).Truncate(time.Second)
	if bytes.Equal(onDisk, current) {
		return fmt.Sprintf("%d bytes, %s ago, current", len(onDisk), age)
	}
	return fmt.Sprintf("%d bytes, %s ago, DIFFERS from the board — any tool call rewrites it",
		len(onDisk), age)
}

func onDiskModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func field(w io.Writer, name, value string) {
	fmt.Fprintf(w, "%-12s %s\n", name, value)
}

func scalar(handle *sql.DB, query string) (string, error) {
	var value string
	if err := handle.QueryRow(query).Scan(&value); err != nil {
		return "", fmt.Errorf("%s: %w", query, err)
	}
	return value, nil
}
