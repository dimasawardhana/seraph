package board

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"seraph/internal/repo"
)

const (
	KanbanFile = "KANBAN.md"
	JSONFile   = "board.json"
)

func Sync(handle *sql.DB, r repo.Root) (Snapshot, []byte, error) {
	tasks, err := Load(handle)
	if err != nil {
		return Snapshot{}, nil, err
	}
	snapshot := New(tasks)

	markdown := snapshot.Markdown()
	encoded, err := snapshot.JSON()
	if err != nil {
		return Snapshot{}, nil, err
	}

	if err := writeAtomic(filepath.Join(r.StatePath(), KanbanFile), markdown); err != nil {
		return Snapshot{}, nil, err
	}
	if err := writeAtomic(filepath.Join(r.StatePath(), JSONFile), encoded); err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, markdown, nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".seraph-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename onto %s: %w", path, err)
	}
	return nil
}
