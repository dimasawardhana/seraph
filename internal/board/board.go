package board

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"seraph/internal/vocab"
)

type Task struct {
	ID           string
	Title        string
	Goal         string
	Description  string
	Acceptance   string
	Status       string
	Triage       string
	Priority     string
	ClaimHarness string
	ClaimSession string
	ClaimExpires int64
	CreatedAt    int64
	UpdatedAt    int64
}

func (t Task) Claimed() bool { return t.ClaimHarness != "" }

const taskColumns = `id, title, COALESCE(goal, ''), COALESCE(description, ''),
	COALESCE(acceptance, ''), status, COALESCE(triage, ''),
	priority, COALESCE(claim_harness, ''), COALESCE(claim_session, ''),
	COALESCE(claim_expires, 0), created_at, updated_at`

func Load(handle *sql.DB) ([]Task, error) {
	rows, err := handle.Query("SELECT " + taskColumns + " FROM tasks")
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Goal, &t.Description, &t.Acceptance,
			&t.Status, &t.Triage, &t.Priority, &t.ClaimHarness, &t.ClaimSession,
			&t.ClaimExpires, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	Sort(tasks)
	return tasks, nil
}

func Sort(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		if sa, sb := vocab.Order(vocab.Statuses, a.Status), vocab.Order(vocab.Statuses, b.Status); sa != sb {
			return sa < sb
		}
		if pa, pb := vocab.Order(vocab.Priorities, a.Priority), vocab.Order(vocab.Priorities, b.Priority); pa != pb {
			return pa < pb
		}
		return a.CreatedAt < b.CreatedAt
	})
}

type Group struct {
	Status string
	Label  string
	Tasks  []Task
}

func Grouped(tasks []Task) []Group {
	var groups []Group
	for _, status := range vocab.Statuses {
		var members []Task
		for _, t := range tasks {
			if t.Status == status.Value {
				members = append(members, t)
			}
		}
		if len(members) > 0 {
			groups = append(groups, Group{Status: status.Value, Label: status.Label, Tasks: members})
		}
	}
	return groups
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
