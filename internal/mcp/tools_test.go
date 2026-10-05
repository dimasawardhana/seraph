package mcp

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"seraph/internal/db"

	"seraph/internal/repo"
)

// newClaimableBoard builds a real board with one claimable task, using seraph's own
// migrations for the same reason the gate tests do.
func newClaimableBoard(t *testing.T) (*sql.DB, repo.Root) {
	t.Helper()

	root := repo.Root{Path: t.TempDir(), Marker: repo.StateDir}
	handle, err := db.Open(root)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	now := time.Now().UTC().Unix()
	if _, err := handle.Exec(
		`INSERT INTO tasks (id, title, goal, acceptance, status, priority, created_at, updated_at)
		 VALUES ('TASK-1', 'A task', 'A goal', 'An acceptance', 'backlog', 'medium', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	return handle, root
}

func claimResult(t *testing.T, handle *sql.DB, root repo.Root, session string) string {
	t.Helper()

	result, _, err := claimTask(handle, root)(context.Background(), nil,
		claimArgs{TaskID: "TASK-1", HarnessID: "test", SessionID: session})
	if err != nil {
		t.Fatalf("claim_task returned a protocol error: %v", err)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	return content.Text
}

// A claim taken under one id while the gate reads another is the failure this warns
// about. It must still succeed — refusing would lock out every caller that cannot supply
// the ambient value — but the caller must be told, rather than finding out later when a
// gate refuses a write it was entitled to make.
func TestClaimWarnsWhenTheSessionIdDisagreesWithTheEnvironment(t *testing.T) {
	handle, root := newClaimableBoard(t)
	t.Setenv("SERAPH_SESSION_ID", "the-id-the-gate-reads")

	got := claimResult(t, handle, root, "a-different-id")

	if !strings.Contains(got, "a-different-id") {
		t.Errorf("expected the warning to name the id the claim was taken under, got:\n%s", got)
	}
	if !strings.Contains(got, "the-id-the-gate-reads") {
		t.Errorf("expected the warning to name the id the gate reads, got:\n%s", got)
	}
	if !strings.Contains(got, "refused as though you held nothing") {
		t.Errorf("expected the warning to state the consequence, got:\n%s", got)
	}
}

func TestClaimIsSilentWhenTheIdsAgree(t *testing.T) {
	handle, root := newClaimableBoard(t)
	t.Setenv("SERAPH_SESSION_ID", "the-same-id")

	got := claimResult(t, handle, root, "the-same-id")

	if strings.Contains(got, "seraph: this claim was taken under session") {
		t.Errorf("no warning when the ids agree, got:\n%s", got)
	}
}

func TestClaimIsSilentWhenTheEnvironmentIsUnset(t *testing.T) {
	handle, root := newClaimableBoard(t)
	t.Setenv("SERAPH_SESSION_ID", "")

	got := claimResult(t, handle, root, "whatever-the-caller-typed")

	if strings.Contains(got, "seraph: this claim was taken under session") {
		t.Errorf("a harness that cannot supply an ambient value must not be scolded, got:\n%s", got)
	}
}
