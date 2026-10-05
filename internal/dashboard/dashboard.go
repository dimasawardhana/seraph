package dashboard

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"seraph/internal/board"
	"seraph/internal/db"
	"seraph/internal/repo"
	"seraph/internal/vocab"
)

const HarnessName = "human"

type Board struct {
	handle  *sql.DB
	root    repo.Root
	session string
}

type Session struct {
	Harness string
	ID      string
}

func (b *Board) Session() Session {
	return Session{Harness: HarnessName, ID: b.session}
}

func Open(root repo.Root) (*Board, error) {
	handle, err := db.Open(root)
	if err != nil {
		return nil, err
	}
	token, err := randomID()
	if err != nil {
		handle.Close()
		return nil, err
	}
	return &Board{handle: handle, root: root, session: token}, nil
}

func (b *Board) Close() error { return b.handle.Close() }

func (b *Board) Root() repo.Root { return b.root }

func randomID() (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

type Item struct {
	ID           string
	Title        string
	Goal         string
	Description  string
	Acceptance   string
	Status       string
	Triage       string
	Priority     string
	ClaimHolder  string
	ClaimExpires int64
	ClaimedByMe  bool
	UpdatedAt    int64
}

type Column struct {
	Status string
	Label  string
	Items  []Item
}

type View struct {
	Columns []Column
	Total   int
	Root    string
}

func (b *Board) View() (View, error) {
	tasks, err := board.Load(b.handle)
	if err != nil {
		return View{}, err
	}

	view := View{Root: b.root.Path, Total: len(tasks)}
	for _, group := range board.Grouped(tasks) {
		column := Column{Status: group.Status, Label: group.Label}
		for _, t := range group.Tasks {
			column.Items = append(column.Items, b.toItem(t))
		}
		view.Columns = append(view.Columns, column)
	}
	return view, nil
}

func (b *Board) toItem(t board.Task) Item {
	return Item{
		ID:           t.ID,
		Title:        t.Title,
		Goal:         t.Goal,
		Description:  t.Description,
		Acceptance:   t.Acceptance,
		Status:       t.Status,
		Triage:       t.Triage,
		Priority:     t.Priority,
		ClaimHolder:  t.ClaimHarness,
		ClaimExpires: t.ClaimExpires,
		ClaimedByMe:  t.ClaimHarness != "" && t.ClaimSession == b.session,
		UpdatedAt:    t.UpdatedAt,
	}
}

func (b *Board) Claim(id string) error {
	_, _, err := board.ClaimTask(b.handle, b.root, board.ClaimParams{
		TaskID:  id,
		Harness: HarnessName,
		Session: b.session,
	})
	return err
}

func (b *Board) Release(id string) error {
	_, _, err := board.ReleaseTask(b.handle, b.root, board.ReleaseParams{
		TaskID:  id,
		Session: b.session,
	})
	return err
}

func (b *Board) SetStatus(id, status string) error {
	_, _, err := board.UpdateTask(b.handle, b.root, board.UpdateParams{
		TaskID:  id,
		Session: b.session,
		Status:  &status,
	})
	return err
}

func (b *Board) SetTriage(id, triage string) error {
	_, _, err := board.UpdateTask(b.handle, b.root, board.UpdateParams{
		TaskID:  id,
		Session: b.session,
		Triage:  &triage,
	})
	return err
}

func (b *Board) Create(title, goal, description, acceptance, priority, triage string) (string, error) {
	tasks, err := board.Load(b.handle)
	if err != nil {
		return "", err
	}
	_, _, err = board.Create(b.handle, b.root, board.CreateParams{
		Title:       title,
		Goal:        goal,
		Description: description,
		Acceptance:  acceptance,
		Priority:    priority,
		Triage:      triage,
	})
	if err != nil {
		return "", err
	}
	after, err := board.Load(b.handle)
	if err != nil || len(after) <= len(tasks) {
		return "", err
	}
	newest := after[0]
	for _, t := range after {
		if t.CreatedAt > newest.CreatedAt {
			newest = t
		}
	}
	return newest.ID, nil
}

func HumanHolders(view View) int {
	n := 0
	for _, c := range view.Columns {
		for _, i := range c.Items {
			if i.ClaimedByMe {
				n++
			}
		}
	}
	return n
}

func Expiry(expires int64) string {
	if expires == 0 {
		return ""
	}
	remaining := time.Until(time.Unix(expires, 0))
	if remaining <= 0 {
		return "expired"
	}
	return "in " + remaining.Truncate(time.Minute).String()
}

func StatusLabel(status string) string { return vocab.Label(vocab.Statuses, status) }

func NextStatus(status string) string {
	for i, o := range vocab.Statuses {
		if o.Value == status && i > 0 {
			return vocab.Statuses[i-1].Value
		}
	}
	return status
}
