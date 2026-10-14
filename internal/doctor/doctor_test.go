package doctor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seraph/internal/board"
	"seraph/internal/db"
	"seraph/internal/repo"
)

func TestMissingReferencesNamesTheTaskAndThePath(t *testing.T) {
	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	// One document exists, so the reference to it must not be reported.
	if err := os.MkdirAll(filepath.Join(root.Path, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root.Path, "docs", "real.md"), []byte("# real\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, err := board.Create(handle, root, board.CreateParams{
		Title:      "Points at two documents",
		Goal:       "One of them is missing",
		Acceptance: "1. doctor says so",
		References: []string{"docs/real.md", "docs/gone.md#section"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	got := missingReferences(handle, root)

	if len(got) != 1 {
		t.Fatalf("expected exactly one missing reference, got %v", got)
	}
	// The anchor is dropped: whether the file carries that heading is a different question,
	// and claiming to check it would be the same defect one level in.
	if got[0] != "TASK-101 → docs/gone.md is missing" {
		t.Errorf("got %q", got[0])
	}
}

func TestMissingReferencesIsQuietWhenEveryPathExists(t *testing.T) {
	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if err := os.WriteFile(filepath.Join(root.Path, "CONTEXT.md"), []byte("# ctx\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := board.Create(handle, root, board.CreateParams{
		Title:      "Points at a document that is there",
		Goal:       "Nothing to report",
		Acceptance: "1. silence",
		References: []string{"CONTEXT.md#index"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if got := missingReferences(handle, root); len(got) != 0 {
		t.Errorf("expected no warnings, got %v", got)
	}
}

func TestSnapshotStateNamesBuildSkewAsSkew(t *testing.T) {
	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if _, _, err := board.Create(handle, root, board.CreateParams{
		Title:      "One task",
		Goal:       "So the snapshot is not empty",
		Acceptance: "1. renders",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A snapshot written by an older build: current content, wrong recorded format.
	stale := board.New(mustTasks(t, handle))
	old := append([]byte(nil), stale.Markdown()...)
	old = replaceFormat(t, old, board.RenderFormat-1)
	path := filepath.Join(root.StatePath(), board.KanbanFile)
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	got := snapshotState(handle, root, board.KanbanFile)
	if !strings.Contains(got, "build skew") || !strings.Contains(got, "restart") {
		t.Fatalf("expected skew named with its remedy, got %q", got)
	}
	if strings.Contains(got, "any tool call rewrites it") {
		t.Fatalf("skew must not be reported as staleness, got %q", got)
	}
}

func TestSnapshotStateNamesFormatlessDifferenceAsStale(t *testing.T) {
	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if _, _, err := board.Create(handle, root, board.CreateParams{
		Title:      "One task",
		Goal:       "So the snapshot is not empty",
		Acceptance: "1. renders",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The board changed since the snapshot was written, and the snapshot predates the
	// format field: an honest unknown, reported as staleness.
	path := filepath.Join(root.StatePath(), board.KanbanFile)
	if err := os.WriteFile(path, []byte("# Board\n\nold render\n"), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	got := snapshotState(handle, root, board.KanbanFile)
	if !strings.Contains(got, "DIFFERS from the board") {
		t.Fatalf("expected staleness, got %q", got)
	}
}

func TestSnapshotStateIsCurrentWhenBytesMatch(t *testing.T) {
	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	if _, _, err := board.Create(handle, root, board.CreateParams{
		Title:      "One task",
		Goal:       "So the snapshot is not empty",
		Acceptance: "1. renders",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	snapshot := board.New(mustTasks(t, handle))
	path := filepath.Join(root.StatePath(), board.KanbanFile)
	if err := os.WriteFile(path, snapshot.Markdown(), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	if got := snapshotState(handle, root, board.KanbanFile); !strings.Contains(got, "current") {
		t.Fatalf("expected current, got %q", got)
	}
}

func mustTasks(t *testing.T, handle *sql.DB) []board.Task {
	t.Helper()
	tasks, err := board.Load(handle)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return tasks
}

// replaceFormat rewrites the format line of a rendered markdown snapshot.
func replaceFormat(t *testing.T, data []byte, format int) []byte {
	t.Helper()
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "_Render format ") {
			lines[i] = fmt.Sprintf("_Render format %d._", format)
			return []byte(strings.Join(lines, "\n"))
		}
	}
	t.Fatalf("no format line in %q", data)
	return nil
}
