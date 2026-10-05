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

func (s Snapshot) Markdown() []byte {
	groups := Grouped(s.Tasks)

	var buf bytes.Buffer
	buf.WriteString(header)
	if len(groups) == 0 {
		buf.WriteString("\nNo tasks yet.\n")
		return buf.Bytes()
	}

	for _, group := range groups {
		fmt.Fprintf(&buf, "\n## %s\n\n", group.Label)
		for _, t := range group.Tasks {
			writeTask(&buf, t)
		}
	}
	return buf.Bytes()
}

func writeTask(buf *bytes.Buffer, t Task) {
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
		lines = append(lines, string(bytes.TrimRight(line, "\r")))
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
