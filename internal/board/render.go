package board

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type Snapshot struct {
	Tasks []Task
}

func New(tasks []Task) Snapshot { return Snapshot{Tasks: tasks} }

const header = "# Board\n\n" +
	"_Written by Seraph. Do not edit — this file is replaced on every change._\n"

// MarkdownOpts narrows a render. The zero value renders every open task in full and every
// closed task compactly, which is what the committed artifacts carry.
//
// This exists to control a cost that was measured rather than guessed. Every mutating tool
// returns the board, and the board carried every closed task's full acceptance prose, so
// the price of recording one task grew with the project's entire history: on this
// repository, 132 KB per write of which 97% was done tasks, against 4.4 KB of actual open
// work. A closed task is history; it earns its heading and its goal here, and its full
// record on demand through get_task.
type MarkdownOpts struct {
	// Statuses limits the render to these statuses. Empty means every status.
	Statuses []string
	// FullDone renders closed tasks in full instead of compactly, for a caller reading
	// history rather than dispatching work.
	FullDone bool
	// OmitDone drops closed tasks entirely, for a caller that wants only the work left.
	OmitDone bool
}

func (s Snapshot) Markdown() []byte { return s.Render(MarkdownOpts{}) }

func (s Snapshot) Render(opts MarkdownOpts) []byte {
	tasks := s.Tasks
	if len(opts.Statuses) > 0 || opts.OmitDone {
		kept := make([]Task, 0, len(tasks))
		for _, t := range tasks {
			if opts.OmitDone && t.Status == done {
				continue
			}
			if len(opts.Statuses) > 0 && !hasStatus(opts.Statuses, t.Status) {
				continue
			}
			kept = append(kept, t)
		}
		tasks = kept
	}

	groups := Grouped(tasks)

	var buf bytes.Buffer
	buf.WriteString(header)
	if len(groups) == 0 {
		buf.WriteString("\nNo tasks yet.\n")
		return buf.Bytes()
	}

	for _, group := range groups {
		fmt.Fprintf(&buf, "\n## %s\n\n", group.Label)
		for _, t := range group.Tasks {
			writeTask(&buf, t, opts.FullDone)
		}
	}
	return buf.Bytes()
}

// done is the terminal status. Named here rather than inlined so a later rename of the
// vocabulary cannot leave this render compacting the wrong group.
const done = "done"

func hasStatus(statuses []string, status string) bool {
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}

// writeTask renders one task.
//
// Everything here is a pure function of the Task. Nothing reads the filesystem, and that is
// load-bearing rather than incidental: the rendered board must be byte-identical on every
// machine holding the same database, or a clone would report its snapshots as differing from
// a board it had never touched. That is also why a reference whose file is missing is
// reported by `seraph doctor` and never here.
func writeTask(buf *bytes.Buffer, t Task, fullDone bool) {
	// A closed task keeps its heading and its goal, and drops the two fields that carry
	// prose: acceptance and description. Those are where the weight was — twenty of the
	// thirty-three tasks on this board carried more than 2 KB of acceptance text, and the
	// closed ones were 134 KB of a 138 KB board. Nothing is lost by not repeating them on
	// every call: the full record stays in the database and comes back from get_task by id.
	//
	// The heading and goal are kept rather than reduced to one bullet, because a reader
	// scanning KANBAN.md for what got done needs to find `### TASK-121` and read what it
	// was for. A bare one-liner list would have made Done unreadable to save a few KB that
	// are not the problem.
	if t.Status == done && !fullDone {
		fmt.Fprintf(buf, "### %s · %s\n\n", t.ID, oneLine(t.Title))
		if t.Goal != "" {
			fmt.Fprintf(buf, "> **Goal:** %s\n", oneLine(t.Goal))
		}
		fmt.Fprintf(buf, "- priority: %s\n", t.Priority)
		fmt.Fprintf(buf, "- updated: %s\n\n", stamp(t.UpdatedAt))
		return
	}

	fmt.Fprintf(buf, "### %s · %s\n\n", t.ID, oneLine(t.Title))
	if t.Goal != "" {
		for _, line := range splitLines(t.Goal) {
			if line != "" {
				fmt.Fprintf(buf, "> **Goal:** %s\n", line)
			}
		}
	}

	fmt.Fprintf(buf, "- priority: %s\n", t.Priority)
	if t.Triage != "" {
		fmt.Fprintf(buf, "- triage: %s\n", t.Triage)
	}
	if t.Claimed() {
		fmt.Fprintf(buf, "- claimed by: %s (session `%s`, until %s)\n",
			oneLine(t.ClaimHarness), t.ClaimSession, stamp(t.ClaimExpires))
	}
	fmt.Fprintf(buf, "- updated: %s\n", stamp(t.UpdatedAt))

	if len(t.References) > 0 {
		fmt.Fprintf(buf, "references:\n")
		for _, ref := range t.References {
			fmt.Fprintf(buf, "  - %s\n", oneLine(ref))
		}
	}

	if t.Acceptance != "" {
		buf.WriteString("\n")
		for _, line := range splitLines(t.Acceptance) {
			if line != "" {
				fmt.Fprintf(buf, "> **Done when:** %s\n", line)
			}
		}
	}
	if t.Description != "" {
		buf.WriteString("\n")
		for _, line := range splitLines(t.Description) {
			if line != "" {
				fmt.Fprintf(buf, "> %s\n", line)
			}
		}
	}
	buf.WriteString("\n")
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range bytes.Split([]byte(s), []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		lines = append(lines, string(line))
	}
	return lines
}

type boardJSON struct {
	Tasks []taskJSON `json:"tasks"`
}

type taskJSON struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Goal        string     `json:"goal"`
	Description string     `json:"description,omitempty"`
	Acceptance  string     `json:"acceptance,omitempty"`
	Status      string     `json:"status"`
	Triage      string     `json:"triage,omitempty"`
	Priority    string     `json:"priority"`
	References  []string   `json:"references,omitempty"`
	Claim       *claimJSON `json:"claim"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type claimJSON struct {
	Harness string `json:"harness"`
	Session string `json:"session"`
	Expires string `json:"expires"`
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("render json: %w", err)
	}
	return buf.Bytes(), nil
}

func (s Snapshot) JSON() ([]byte, error) {
	out := boardJSON{Tasks: make([]taskJSON, 0, len(s.Tasks))}
	for _, t := range s.Tasks {
		out.Tasks = append(out.Tasks, taskToJSON(t))
	}
	return encodeJSON(out)
}

func taskToJSON(t Task) taskJSON {
	entry := taskJSON{
		ID:          t.ID,
		Title:       t.Title,
		Goal:        t.Goal,
		Description: t.Description,
		Acceptance:  t.Acceptance,
		Status:      t.Status,
		Triage:      t.Triage,
		Priority:    t.Priority,
		References:  t.References,
		CreatedAt:   stamp(t.CreatedAt),
		UpdatedAt:   stamp(t.UpdatedAt),
	}
	if t.Claimed() {
		entry.Claim = &claimJSON{
			Harness: t.ClaimHarness,
			Session: t.ClaimSession,
			Expires: stamp(t.ClaimExpires),
		}
	}
	return entry
}

func stamp(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}
