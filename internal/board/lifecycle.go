package board

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"seraph/internal/repo"
	"seraph/internal/vocab"
)

var ClaimTTL = 30 * time.Minute

func ConfigureFromEnv() {
	if raw := os.Getenv("SERAPH_CLAIM_TTL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			ClaimTTL = d
		}
	}
}

type CreateParams struct {
	Title       string
	Goal        string
	Description string
	Acceptance  string
	Priority    string
	Triage      string
	References  []string
}

type ClaimParams struct {
	TaskID  string
	Harness string
	Session string
}

type UpdateParams struct {
	TaskID      string
	Session     string
	Status      *string
	Title       *string
	Goal        *string
	Description *string
	Acceptance  *string
	Priority    *string
	Triage      *string
	References  *[]string
}

type ReleaseParams struct {
	TaskID  string
	Session string
}

type claimState struct {
	harness string
	session string
	expires int64
}

func (c claimState) live(now int64) bool { return c.harness != "" && c.expires > now }

func (c claimState) heldBy(session string, now int64) bool {
	return c.live(now) && c.session == session
}

func (c claimState) holder() *Holder {
	if c.harness == "" {
		return nil
	}
	return &Holder{Harness: c.harness, Session: c.session, Expires: c.expires}
}

func mutate(handle *sql.DB, r repo.Root, fn func(*sql.Tx) error) (Snapshot, []byte, error) {
	tx, err := handle.Begin()
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return Snapshot{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, nil, fmt.Errorf("commit: %w", err)
	}
	return Sync(handle, r)
}

func checkTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", &Error{Reason: ReasonInvalidTitle, Message: "a task needs a non-empty title"}
	}
	return title, nil
}

// checkSpec enforces that a task states what it is for and how anyone will know it is
// finished. Without both, a board records that work happened but not whether it was the
// right work — which is the failure this whole mechanism exists to prevent.
func checkSpec(goal, acceptance string) (string, string, error) {
	goal = strings.TrimSpace(goal)
	acceptance = strings.TrimSpace(acceptance)
	if goal == "" {
		return "", "", &Error{Reason: ReasonInvalidGoal,
			Message: "a task needs a goal — one sentence on what changes for whom when it is done"}
	}
	if acceptance == "" {
		return "", "", &Error{Reason: ReasonInvalidAcceptance,
			Message: "a task needs acceptance criteria — how anyone can tell it was done correctly"}
	}
	return goal, acceptance, nil
}

func checkStatus(status string) error {
	if !vocab.Has(vocab.Statuses, status) {
		return &Error{Reason: ReasonInvalidStatus,
			Message: fmt.Sprintf("%q is not a status; expected one of %s", status, join(vocab.Statuses))}
	}
	return nil
}

func checkPriority(priority string) error {
	if priority == "" || vocab.Has(vocab.Priorities, priority) {
		return nil
	}
	return &Error{Reason: ReasonInvalidPriority,
		Message: fmt.Sprintf("%q is not a priority; expected one of %s", priority, join(vocab.Priorities))}
}

func checkTriage(triage string) error {
	if triage == "" || vocab.Has(vocab.Triages, triage) {
		return nil
	}
	return &Error{Reason: ReasonInvalidTriage,
		Message: fmt.Sprintf("%q is not a triage label; expected one of %s", triage, join(vocab.Triages))}
}

func join(options []vocab.Option) string {
	return strings.Join(vocab.Values(options), ", ")
}

func Create(handle *sql.DB, r repo.Root, in CreateParams) (Snapshot, []byte, error) {
	title, err := checkTitle(in.Title)
	if err != nil {
		return Snapshot{}, nil, err
	}
	goal, acceptance, err := checkSpec(in.Goal, in.Acceptance)
	if err != nil {
		return Snapshot{}, nil, err
	}
	if err := checkPriority(in.Priority); err != nil {
		return Snapshot{}, nil, err
	}
	if err := checkTriage(in.Triage); err != nil {
		return Snapshot{}, nil, err
	}

	references, err := NormalizeReferences(in.References)
	if err != nil {
		return Snapshot{}, nil, err
	}

	priority := in.Priority
	if priority == "" {
		priority = vocab.Default(vocab.Priorities)
	}

	return mutate(handle, r, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow("UPDATE id_sequence SET last = last + 1 WHERE id = 1 RETURNING last").Scan(&n); err != nil {
			return fmt.Errorf("allocate task id: %w", err)
		}

		now := time.Now().UTC().Unix()
		_, err := tx.Exec(`INSERT INTO tasks
		                   (id, title, goal, description, acceptance, status, priority, triage, doc_refs, created_at, updated_at)
		                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("TASK-%d", n), title, goal, in.Description, acceptance,
			vocab.Default(vocab.Statuses), priority, nullString(in.Triage),
			encodeReferences(references), now, now)
		if err != nil {
			return fmt.Errorf("insert task: %w", err)
		}
		return nil
	})
}

func ClaimTask(handle *sql.DB, r repo.Root, in ClaimParams) (Snapshot, []byte, error) {
	return mutate(handle, r, func(tx *sql.Tx) error {
		current, err := loadClaim(tx, in.TaskID)
		if err != nil {
			return err
		}

		now := time.Now().UTC().Unix()
		if current.live(now) {
			return alreadyClaimed(in, current)
		}

		expires := now + int64(ClaimTTL.Seconds())
		_, err = tx.Exec(`UPDATE tasks
		                  SET claim_harness = ?, claim_session = ?, claim_expires = ?, updated_at = ?
		                  WHERE id = ?`,
			in.Harness, in.Session, expires, now, in.TaskID)
		if err != nil {
			return fmt.Errorf("claim task: %w", err)
		}
		return nil
	})
}

func alreadyClaimed(in ClaimParams, current claimState) error {
	if current.session != in.Session {
		return &Error{Reason: ReasonHeld,
			Message: fmt.Sprintf("%s is claimed by another session until %s; do not start this work",
				in.TaskID, stamp(current.expires)),
			Holder: current.holder()}
	}

	message := fmt.Sprintf("%s is already claimed by this session until %s; keep working on it",
		in.TaskID, stamp(current.expires))
	if current.harness != in.Harness {
		message += fmt.Sprintf(
			" — the board records this claim under harness %q while you called yourself %q, so two callers are sharing one session id; generate a fresh session_id per run",
			current.harness, in.Harness)
	}
	return &Error{Reason: ReasonAlreadyHeld, Message: message, Holder: current.holder()}
}

func UpdateTask(handle *sql.DB, r repo.Root, in UpdateParams) (Snapshot, []byte, error) {
	var title string
	if in.Title != nil {
		var err error
		if title, err = checkTitle(*in.Title); err != nil {
			return Snapshot{}, nil, err
		}
	}
	if in.Status != nil {
		if err := checkStatus(*in.Status); err != nil {
			return Snapshot{}, nil, err
		}
	}
	if in.Priority != nil {
		if err := checkPriority(*in.Priority); err != nil {
			return Snapshot{}, nil, err
		}
	}
	if in.Triage != nil {
		if err := checkTriage(*in.Triage); err != nil {
			return Snapshot{}, nil, err
		}
	}

	return mutate(handle, r, func(tx *sql.Tx) error {
		current, err := loadClaim(tx, in.TaskID)
		if err != nil {
			return err
		}

		now := time.Now().UTC().Unix()
		completing := in.Status != nil && *in.Status == "done"
		holding := current.heldBy(in.Session, now)

		if completing && !holding {
			return &Error{Reason: ReasonClaimRequired,
				Message: fmt.Sprintf("%s can only be completed by the session holding its claim; call claim_task first", in.TaskID),
				Holder:  current.holder()}
		}

		sets := []string{"updated_at = ?"}
		args := []any{now}
		add := func(column string, value any) {
			sets = append(sets, column+" = ?")
			args = append(args, value)
		}

		if in.Title != nil {
			add("title", title)
		}
		if in.Status != nil {
			add("status", *in.Status)
		}
		if in.Goal != nil {
			goal := strings.TrimSpace(*in.Goal)
			if goal == "" {
				return &Error{Reason: ReasonInvalidGoal, Message: "a task needs a goal"}
			}
			add("goal", goal)
		}
		if in.Description != nil {
			add("description", *in.Description)
		}
		if in.Acceptance != nil {
			acceptance := strings.TrimSpace(*in.Acceptance)
			if acceptance == "" {
				return &Error{Reason: ReasonInvalidAcceptance, Message: "a task needs acceptance criteria"}
			}
			add("acceptance", acceptance)
		}
		if in.Priority != nil && *in.Priority != "" {
			add("priority", *in.Priority)
		}
		if in.Triage != nil {
			add("triage", nullString(*in.Triage))
		}
		if in.References != nil {
			normalized, err := NormalizeReferences(*in.References)
			if err != nil {
				return err
			}
			add("doc_refs", encodeReferences(normalized))
		}

		switch {
		case completing:
			sets = append(sets, "claim_harness = NULL", "claim_session = NULL", "claim_expires = NULL")
		case holding:
			add("claim_expires", now+int64(ClaimTTL.Seconds()))
		}

		args = append(args, in.TaskID)
		if _, err := tx.Exec("UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
			return fmt.Errorf("update task: %w", err)
		}
		return nil
	})
}

func ReleaseTask(handle *sql.DB, r repo.Root, in ReleaseParams) (Snapshot, []byte, error) {
	return mutate(handle, r, func(tx *sql.Tx) error {
		current, err := loadClaim(tx, in.TaskID)
		if err != nil {
			return err
		}
		if current.harness == "" {
			return &Error{Reason: ReasonNotClaimed,
				Message: fmt.Sprintf("%s is not claimed", in.TaskID)}
		}
		if current.session != in.Session {
			return &Error{Reason: ReasonHeld,
				Message: fmt.Sprintf("the claim on %s is held by another session", in.TaskID),
				Holder:  current.holder()}
		}

		_, err = tx.Exec(`UPDATE tasks
		                  SET claim_harness = NULL, claim_session = NULL,
		                      claim_expires = NULL, updated_at = ?
		                  WHERE id = ?`, time.Now().UTC().Unix(), in.TaskID)
		if err != nil {
			return fmt.Errorf("release task: %w", err)
		}
		return nil
	})
}

func loadClaim(tx *sql.Tx, taskID string) (claimState, error) {
	var c claimState
	err := tx.QueryRow(`SELECT COALESCE(claim_harness, ''), COALESCE(claim_session, ''),
	                          COALESCE(claim_expires, 0)
	                   FROM tasks WHERE id = ?`, taskID).Scan(&c.harness, &c.session, &c.expires)
	if err == sql.ErrNoRows {
		return c, &Error{Reason: ReasonTaskNotFound, Message: fmt.Sprintf("no task %s", taskID)}
	}
	if err != nil {
		return c, fmt.Errorf("read task %s: %w", taskID, err)
	}
	return c, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
