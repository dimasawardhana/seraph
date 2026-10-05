package gate

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"seraph/internal/db"
	"seraph/internal/repo"
)

// The repository had no tests until this file, which meant every claim about gate
// behaviour was verified by hand and then written down. These cover the decision, because
// the decision is the contract: a harness is refused or not on the strength of it.

// task is one row, described the way the gate reads it: an id, and either nobody's claim
// or somebody's, with an expiry.
type task struct {
	id      string
	session string
	expires int64
}

// newBoard builds a real board in a temp directory through seraph's own migrations, so
// these tests meet the same schema and the same WAL-mode database the gate meets in
// production. A hand-written state.db is what produced a false "allow" once already, and
// the lesson from that was recorded rather than rediscovered.
func newBoard(t *testing.T, tasks ...task) string {
	t.Helper()

	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	defer handle.Close()

	now := time.Now().UTC().Unix()
	for _, row := range tasks {
		var session, expires any
		if row.session != "" {
			session, expires = row.session, row.expires
		}
		if _, err := handle.Exec(
			`INSERT INTO tasks (id, title, status, priority, claim_session, claim_expires, created_at, updated_at)
			 VALUES (?, ?, 'backlog', 'medium', ?, ?, ?, ?)`,
			row.id, row.id, session, expires, now, now); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	return root.Path
}

func TestWriteAllowedWhenThisSessionHoldsTheClaim(t *testing.T) {
	root := newBoard(t, task{id: "TASK-1", session: "s1", expires: time.Now().Add(time.Hour).Unix()})

	got := Evaluate(root, "Write", "s1", nil)

	if !got.Gate.Allow {
		t.Fatalf("expected allow, got refusal: %s", got.Gate.Reason)
	}
	if got.Gate.Task != "TASK-1" {
		t.Errorf("expected the held task named, got %q", got.Gate.Task)
	}
}

func TestWriteRefusedWhenThisSessionHoldsNothing(t *testing.T) {
	root := newBoard(t, task{id: "TASK-1"})

	got := Evaluate(root, "Write", "s1", nil)

	if got.Gate.Allow {
		t.Fatal("expected a refusal for a session holding no claim")
	}
	if !strings.Contains(got.Gate.Reason, "holds no task") {
		t.Errorf("expected the reason to say the session holds nothing, got %q", got.Gate.Reason)
	}
}

// The case this whole change exists for. Before it, a session whose claim existed under
// a different identity and a session on an empty board received the same sentence, and
// both read as a policy decision rather than an identity mismatch.
func TestRefusalDistinguishesAClaimHeldByAnotherSession(t *testing.T) {
	heldElsewhere := newBoard(t, task{id: "TASK-1", session: "somebody-else", expires: time.Now().Add(time.Hour).Unix()})
	emptyOfOthers := newBoard(t, task{id: "TASK-1"})

	mismatch := Evaluate(heldElsewhere, "Write", "s1", nil)
	if mismatch.Gate.Allow {
		t.Fatal("expected a refusal when the claim belongs to another session")
	}
	if !strings.Contains(mismatch.Gate.Reason, "held by another session") {
		t.Errorf("expected the refusal to name claims held elsewhere, got %q", mismatch.Gate.Reason)
	}

	// The control matters as much as the case: an unclaimed board must NOT produce the
	// new sentence, or every ordinary refusal would accuse the caller of a mismatch.
	ordinary := Evaluate(emptyOfOthers, "Write", "s1", nil)
	if strings.Contains(ordinary.Gate.Reason, "held by another session") {
		t.Errorf("an unclaimed board must not imply an identity mismatch, got %q", ordinary.Gate.Reason)
	}
}

func TestRefusalCarriesTheSameSentenceToExecTools(t *testing.T) {
	root := newBoard(t, task{id: "TASK-1", session: "somebody-else", expires: time.Now().Add(time.Hour).Unix()})

	got := Evaluate(root, "Bash", "s1", nil)

	if got.Gate.Allow {
		t.Fatal("expected a refusal for an exec tool with no claim")
	}
	if !strings.Contains(got.Gate.Reason, "held by another session") {
		t.Errorf("exec refusals carry the same sentence as write refusals, got %q", got.Gate.Reason)
	}
}

// An expired claim is not a claim. It must read as unclaimed, not as somebody else's.
func TestExpiredClaimCountsAsUnclaimed(t *testing.T) {
	past := time.Now().Add(-time.Minute).Unix()
	root := newBoard(t, task{id: "TASK-1", session: "somebody-else", expires: past})

	got := Evaluate(root, "Write", "s1", nil)

	if got.Gate.Allow {
		t.Fatal("expected a refusal: the only claim has expired")
	}
	if !strings.Contains(got.Gate.Reason, "1 task(s) are unclaimed") {
		t.Errorf("expected the expired claim to be offered as unclaimed, got %q", got.Gate.Reason)
	}
	if strings.Contains(got.Gate.Reason, "held by another session") {
		t.Errorf("an expired claim is not held by anyone, got %q", got.Gate.Reason)
	}
}

// With no session the gate cannot attribute a claim, and refusing there would be a guess.
// It allows instead — and must keep doing so now that elsewhere is counted.
func TestUnattributableClaimAllowsRatherThanGuesses(t *testing.T) {
	root := newBoard(t, task{id: "TASK-1", session: "somebody-else", expires: time.Now().Add(time.Hour).Unix()})

	got := Evaluate(root, "Write", "", nil)

	if !got.Gate.Allow {
		t.Fatalf("expected allow when no session can be identified, got %q", got.Gate.Reason)
	}
	if !strings.Contains(got.Reason, "cannot tell which") {
		t.Errorf("expected the reason to admit it cannot attribute the claim, got %q", got.Reason)
	}
}

// A gate that obstructs the mechanism satisfying it is a deadlock, not a rule. This
// happened once: "create" matched the write tools and refused mcp__seraph_create_task.
func TestSeraphOwnToolsAreNeverGated(t *testing.T) {
	root := newBoard(t)

	for _, tool := range []string{"mcp__seraph_create_task", "mcp__seraph_claim_task", "xd://mcp__seraph_release_task"} {
		if got := Evaluate(root, tool, "", nil); !got.Gate.Allow {
			t.Errorf("%s must never be gated, got refusal: %s", tool, got.Gate.Reason)
		}
	}

	// And the same call arriving through a harness's own write tool, aimed at the device.
	input := map[string]any{"path": "xd://mcp__seraph_get_board_summary"}
	if got := Evaluate(root, "Write", "", input); !got.Gate.Allow {
		t.Errorf("a write aimed at a seraph device must not be gated, got refusal: %s", got.Gate.Reason)
	}
}

func TestReadToolsAreNeverGated(t *testing.T) {
	root := newBoard(t)

	for _, tool := range []string{"read", "grep", "glob", "list"} {
		if got := Evaluate(root, tool, "", nil); !got.Gate.Allow {
			t.Errorf("%s must never need a claim, got refusal: %s", tool, got.Gate.Reason)
		}
	}
}

// Seraph writes its own snapshots from inside a mutation. A gate that blocked those would
// deadlock a harness against a board it cannot refresh.
func TestSeraphSnapshotsAreExempt(t *testing.T) {
	for _, path := range []string{
		".seraph/KANBAN.md",
		".seraph/board.json",
		"/repo/.seraph/state.db",
	} {
		if !PathExempt(filepath.FromSlash(path)) {
			t.Errorf("%s must be exempt from the gate", path)
		}
	}
	if PathExempt(filepath.FromSlash("internal/gate/gate.go")) {
		t.Error("an ordinary source file must not be exempt")
	}
}
