package board

import (
	"database/sql"
	"fmt"
)

func Get(handle *sql.DB, taskID string) (Task, error) {
	var t Task
	err := handle.QueryRow("SELECT "+taskColumns+" FROM tasks WHERE id = ?", taskID).Scan(
		&t.ID, &t.Title, &t.Goal, &t.Description, &t.Acceptance, &t.Status, &t.Triage,
		&t.Priority, &t.ClaimHarness, &t.ClaimSession, &t.ClaimExpires,
		&t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return t, &Error{Reason: ReasonTaskNotFound, Message: fmt.Sprintf("no task %s", taskID)}
	}
	if err != nil {
		return t, fmt.Errorf("read task %s: %w", taskID, err)
	}
	return t, nil
}

func JSONDetail(t Task) ([]byte, error) {
	return encodeJSON(taskToJSON(t))
}
