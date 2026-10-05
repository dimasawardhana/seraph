package doctor

import (
	"os"
	"path/filepath"
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
