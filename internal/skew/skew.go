// Package skew detects whether the running executable's file has changed on disk
// since this process started. A serve process renders with its start-time binary, so
// a rebuilt binary reaches an already-running session only at restart (TASK-136); the
// watcher exists so that window is named rather than invisible.
package skew

import (
	"fmt"
	"os"
	"time"
)

// Record is the executable identity captured at startup.
type Record struct {
	path    string
	modTime time.Time
	size    int64
}

// Capture records the running executable's path, mtime and size.
func Capture() (Record, error) {
	path, err := os.Executable()
	if err != nil {
		return Record{}, fmt.Errorf("locate executable: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Record{}, fmt.Errorf("stat executable: %w", err)
	}
	return Record{path: path, modTime: info.ModTime(), size: info.Size()}, nil
}

// Changed reports whether the on-disk executable now differs from the recorded one.
// A file that can no longer be stated counts as changed: whatever replaced it, it is
// not the binary this process is running.
func (r Record) Changed() (bool, string) {
	info, err := os.Stat(r.path)
	if err != nil {
		return true, fmt.Sprintf("on-disk binary %s is no longer statable (%v); a restart is needed to pick up whatever replaced it", r.path, err)
	}
	if !info.ModTime().Equal(r.modTime) || info.Size() != r.size {
		detail := "its modification time changed"
		if info.Size() != r.size {
			detail = fmt.Sprintf("it was %d bytes, now %d", r.size, info.Size())
		}
		return true, fmt.Sprintf("on-disk binary %s changed since this server started (%s); this session still renders with its start-time binary — a restart is needed to pick the rebuild up", r.path, detail)
	}
	return false, ""
}
