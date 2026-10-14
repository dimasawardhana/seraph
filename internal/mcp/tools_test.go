package mcp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"seraph/internal/board"
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

// get_task answered with a Scan error for every id because Get dropped the doc_refs
// column taskColumns selects. This calls the real tool on a real task and asserts the body
// comes back — references included — so the defect cannot return unnoticed.
func TestGetTaskReturnsTheTaskBody(t *testing.T) {
	handle, _ := newClaimableBoard(t)
	if _, err := handle.Exec(`UPDATE tasks SET doc_refs = ? WHERE id = 'TASK-1'`,
		`["CONTEXT.md#index","docs/adr/0001"]`); err != nil {
		t.Fatalf("set references: %v", err)
	}

	result, _, err := getTask(handle)(context.Background(), nil, getTaskArgs{TaskID: "TASK-1"})
	if err != nil {
		t.Fatalf("get_task returned a protocol error: %v", err)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	for _, want := range []string{"TASK-1", "CONTEXT.md#index", "docs/adr/0001"} {
		if !strings.Contains(content.Text, want) {
			t.Errorf("get_task body is missing %q, got:\n%s", want, content.Text)
		}
	}
}

func summaryResult(t *testing.T, handle *sql.DB, root repo.Root, args boardArgs) string {
	t.Helper()

	result, _, err := getBoardSummary(handle, root)(context.Background(), nil, args)
	if err != nil {
		t.Fatalf("get_board_summary returned a protocol error: %v", err)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	return content.Text
}

// The long-standing promise is that this tool's response is the file on disk, byte for
// byte. It is also the expensive path: the response is every tool call's payload, so the
// default must be the rendered board and not a second render that could drift from it.
func TestBoardSummaryIsTheRenderedFile(t *testing.T) {
	handle, root := newClaimableBoard(t)

	got := summaryResult(t, handle, root, boardArgs{})

	onDisk, err := os.ReadFile(filepath.Join(root.StatePath(), board.KanbanFile))
	if err != nil {
		t.Fatalf("read %s: %v", board.KanbanFile, err)
	}
	if got != string(onDisk) {
		t.Errorf("the tool's response is not the rendered file:\n tool: %q\n file: %q", got, onDisk)
	}
}

// status_filter and include_done were recorded in ROADMAP.md Phase 5 as planned and never
// built, so the one tool a dispatcher uses could only ever return everything.
func TestBoardSummaryFiltersByStatus(t *testing.T) {
	handle, root := newClaimableBoard(t)
	if _, err := handle.Exec(`UPDATE tasks SET status = 'done' WHERE id = 'TASK-1'`); err != nil {
		t.Fatalf("close the task: %v", err)
	}
	if _, err := handle.Exec(
		`INSERT INTO tasks (id, title, goal, acceptance, status, priority, created_at, updated_at)
		 VALUES ('TASK-2', 'Open work', 'A goal', 'An acceptance', 'todo', 'medium', 0, 0)`); err != nil {
		t.Fatalf("insert open task: %v", err)
	}

	only := summaryResult(t, handle, root, boardArgs{StatusFilter: []string{"todo"}})

	if !strings.Contains(only, "TASK-2") {
		t.Errorf("the filtered-in status is missing:\n%s", only)
	}
	if strings.Contains(only, "TASK-1") {
		t.Errorf("a status outside the filter leaked in:\n%s", only)
	}
}

// include_done asks for closed work, and a caller that asks wants the record, not the
// compacted line the default render carries.
func TestBoardSummaryIncludeDoneReturnsTheRecord(t *testing.T) {
	handle, root := newClaimableBoard(t)
	if _, err := handle.Exec(
		`UPDATE tasks SET status = 'done', acceptance = 'the detailed acceptance' WHERE id = 'TASK-1'`); err != nil {
		t.Fatalf("close the task: %v", err)
	}

	withDone := summaryResult(t, handle, root, boardArgs{IncludeDone: true})

	if !strings.Contains(withDone, "the detailed acceptance") {
		t.Errorf("include_done must return the full record:\n%s", withDone)
	}
}
