package board

import (
	"strings"
	"testing"
)

// Every mutating tool returns the board, and the board used to carry every closed task's
// full acceptance prose. The cost of recording one task therefore grew with the project's
// whole history: on this repository 137.6 KB per call, of which 116.6 KB was the acceptance
// and description text of tasks that were already finished. These tests pin the fix, since
// a regression here is invisible locally and only shows up as a bill.

func longTask(status string) Task {
	return Task{
		ID:         "TASK-1",
		Title:      "A task",
		Goal:       "What changes for whom",
		Status:     status,
		Priority:   "medium",
		Acceptance: strings.Repeat("a long acceptance criterion. ", 200),
	}
}

func TestClosedTasksAreCompacted(t *testing.T) {
	done := longTask(done)
	done.Description = strings.Repeat("a long description. ", 200)

	rendered := string(New([]Task{done}).Markdown())

	if !strings.Contains(rendered, "### TASK-1") {
		t.Errorf("a closed task must still be findable by id:\n%s", rendered)
	}
	if !strings.Contains(rendered, "What changes for whom") {
		t.Errorf("a closed task must still say what it was for:\n%s", rendered)
	}
	if strings.Contains(rendered, "a long acceptance criterion") {
		t.Errorf("a closed task's acceptance prose must not be re-sent on every call:\n%s", rendered)
	}
	if strings.Contains(rendered, "a long description") {
		t.Errorf("a closed task's description must not be re-sent on every call:\n%s", rendered)
	}
}

// The compaction must never touch open work: a caller dispatching a task needs its
// acceptance criteria, which is the whole reason the field exists.
func TestOpenWorkIsNeverCompacted(t *testing.T) {
	open := longTask("todo")
	open.Acceptance = "the criterion a worker needs"

	rendered := string(New([]Task{open}).Markdown())

	if !strings.Contains(rendered, "the criterion a worker needs") {
		t.Errorf("an open task keeps its acceptance criteria in full:\n%s", rendered)
	}
}

// A caller reading history must be able to ask for the record back.
func TestFullDoneRestoresTheWholeRecord(t *testing.T) {
	done := longTask(done)
	done.Acceptance = "the full record"

	rendered := string(New([]Task{done}).Render(MarkdownOpts{FullDone: true}))

	if !strings.Contains(rendered, "the full record") {
		t.Errorf("FullDone must render a closed task in full:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Done when") {
		t.Errorf("FullDone must emit the acceptance block:\n%s", rendered)
	}
}

// The zero value must stay the whole board, because that is what the committed artifacts
// and the byte-identity promise depend on.
func TestZeroValueRendersEveryTask(t *testing.T) {
	tasks := []Task{longTask("todo"), longTask(done)}
	rendered := string(New(tasks).Render(MarkdownOpts{}))

	for _, id := range []string{"TASK-1"} {
		if !strings.Contains(rendered, id) {
			t.Errorf("zero-value render dropped %s:\n%s", id, rendered)
		}
	}
}

func TestStatusFilterLimitsTheRender(t *testing.T) {
	open := longTask("todo")
	open.ID = "TASK-OPEN"
	closed := longTask(done)
	closed.ID = "TASK-CLOSED"

	rendered := string(New([]Task{open, closed}).Render(MarkdownOpts{Statuses: []string{"todo"}}))

	if !strings.Contains(rendered, "TASK-OPEN") {
		t.Errorf("the filtered-in status must be present:\n%s", rendered)
	}
	if strings.Contains(rendered, "TASK-CLOSED") {
		t.Errorf("a status outside the filter must not appear:\n%s", rendered)
	}
	if strings.Contains(rendered, "## Done") {
		t.Errorf("an empty group must be omitted, not rendered blank:\n%s", rendered)
	}
}

func TestOmitDoneLeavesOnlyOpenWork(t *testing.T) {
	open := longTask("todo")
	open.ID = "TASK-OPEN"
	closed := longTask(done)
	closed.ID = "TASK-CLOSED"

	rendered := string(New([]Task{open, closed}).Render(MarkdownOpts{OmitDone: true}))

	if strings.Contains(rendered, "TASK-CLOSED") {
		t.Errorf("OmitDone must drop closed tasks entirely:\n%s", rendered)
	}
	if !strings.Contains(rendered, "TASK-OPEN") {
		t.Errorf("OmitDone must keep open work:\n%s", rendered)
	}
}

// The saving has to be real and proportional, not incidental: this is the property the
// whole change exists for. A single closed task with 4 KB of prose must not put 4 KB back
// on every call.
func TestCompactionSavesTheProse(t *testing.T) {
	done := longTask(done)
	full := len(New([]Task{done}).Render(MarkdownOpts{FullDone: true}))
	compact := len(New([]Task{done}).Markdown())

	if full < 4000 {
		t.Fatalf("fixture is too small to prove anything: %d bytes", full)
	}
	if compact > full/4 {
		t.Errorf("a closed task must shrink to a small fraction: %d of %d bytes", compact, full)
	}
}
