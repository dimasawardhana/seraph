package skew

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The mechanism TASK-148 pins: a serve process that cannot tell its on-disk binary
// changed lets a rebuild stay invisible for the life of the session. These use a real
// temp file rather than the test binary itself, so the changed and unchanged paths are
// both exercised deterministically.

func TestUnchangedBinaryReportsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seraph")
	if err := os.WriteFile(path, []byte("build-one"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	record := Record{path: path}
	// Stat fills the record from the file as it is now, mirroring Capture.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	record.modTime, record.size = info.ModTime(), info.Size()

	changed, message := record.Changed()
	if changed {
		t.Fatalf("unchanged binary reported changed: %s", message)
	}
}

func TestRebuiltBinaryIsNamedAsBuildSkew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seraph")
	if err := os.WriteFile(path, []byte("build-one"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	record := Record{path: path}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	record.modTime, record.size = info.ModTime(), info.Size()

	// A rebuild replaces the file: different bytes, later mtime.
	if err := os.WriteFile(path, []byte("build-two-longer"), 0o755); err != nil {
		t.Fatalf("rewrite binary: %v", err)
	}
	// Guarantee the mtime moved, since coarse timestamps can collapse two writes.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	changed, message := record.Changed()
	if !changed {
		t.Fatal("rebuilt binary not detected")
	}
	for _, want := range []string{"changed since this server started", "restart"} {
		if !contains(message, want) {
			t.Errorf("message missing %q, got %q", want, message)
		}
	}
}

func TestVanishedBinaryIsReportedAsChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seraph")
	record := Record{path: path, modTime: time.Now(), size: 10}

	changed, message := record.Changed()
	if !changed {
		t.Fatal("vanished binary not reported changed")
	}
	if !contains(message, "no longer statable") {
		t.Errorf("message missing vanish reason, got %q", message)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
