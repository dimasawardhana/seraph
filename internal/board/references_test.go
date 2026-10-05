package board

import (
	"strings"
	"testing"
)

func TestNormalizeReferencesKeepsOrderAndDropsNoise(t *testing.T) {
	got, err := NormalizeReferences([]string{
		"  docs/adr/0004-seraph-indexes-prose-not-code.md  ",
		"",
		"CONTEXT.md#index",
		"CONTEXT.md#index",
		"   ",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := []string{
		"docs/adr/0004-seraph-indexes-prose-not-code.md",
		"CONTEXT.md#index",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// A reference is printed in a file other people read. One that leaves the repository, or
// that names a path on the machine that happened to write it, means nothing to a reader.
func TestNormalizeReferencesRefusesPathsThatOnlyMakeSenseHere(t *testing.T) {
	for _, ref := range []string{"/etc/passwd", "../outside.md", "docs/../../outside.md", "#anchor-only"} {
		if _, err := NormalizeReferences([]string{ref}); err == nil {
			t.Errorf("%q must be refused", ref)
		}
	}
}

func TestReferencePathDropsTheAnchor(t *testing.T) {
	if got := ReferencePath("CONTEXT.md#index"); got != "CONTEXT.md" {
		t.Errorf("got %q, want CONTEXT.md", got)
	}
	if got := ReferencePath("docs/a.md"); got != "docs/a.md" {
		t.Errorf("a reference with no anchor must come back whole, got %q", got)
	}
}

func TestReferencesRoundTripThroughTheDatabase(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, "")

	want := []string{"docs/adr/0001-rendered-view-is-the-product.md", "CONTEXT.md#index"}
	if _, _, err := Create(handle, root, CreateParams{
		Title:      "Points at its paperwork",
		Goal:       "Survive the round trip",
		Acceptance: "1. it comes back",
		References: want,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	tasks, err := Load(handle)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	got := tasks[0].References
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestReferencesAppearInBothSnapshots(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, "")
	_, markdown, err := Create(handle, root, CreateParams{
		Title:      "Points at its paperwork",
		Goal:       "Be visible",
		Acceptance: "1. in both files",
		References: []string{"CONTEXT.md#index"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if !strings.Contains(string(markdown), "- CONTEXT.md#index") {
		t.Errorf("KANBAN.md does not carry the reference:\n%s", markdown)
	}

	tasks, err := Load(handle)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	encoded, err := New(tasks).JSON()
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(string(encoded), `"references": [`) ||
		!strings.Contains(string(encoded), `"CONTEXT.md#index"`) {
		t.Errorf("board.json does not carry the reference array:\n%s", encoded)
	}
}

// A task with no references must render exactly as it did before this field existed.
func TestATaskWithNoReferencesRendersUnchanged(t *testing.T) {
	root, handle := newRepoWithSnapshot(t, "")
	_, markdown, err := Create(handle, root, CreateParams{
		Title:      "Plain",
		Goal:       "Nothing extra",
		Acceptance: "1. one",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if strings.Contains(string(markdown), "references:") {
		t.Errorf("a task with no references must not render an empty block:\n%s", markdown)
	}
}

// A clone restores from board.json, so a reference that does not survive the seed is a
// reference every clone silently loses.
func TestSeedRestoresReferences(t *testing.T) {
	snapshot := `{"tasks":[{"id":"TASK-9","title":"Points at its paperwork","goal":"Survive a clone",
	  "acceptance":"1. it comes back","status":"backlog","priority":"medium",
	  "references":["docs/adr/0001-rendered-view-is-the-product.md","CONTEXT.md#index"],
	  "claim":null,"created_at":"2026-10-05T08:40:43Z","updated_at":"2026-10-05T08:40:43Z"}]}`
	root, handle := newRepoWithSnapshot(t, snapshot)

	if _, err := SeedFromSnapshot(handle, root); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tasks, err := Load(handle)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	if len(tasks[0].References) != 2 || tasks[0].References[1] != "CONTEXT.md#index" {
		t.Errorf("references did not survive the seed: %v", tasks[0].References)
	}
}
