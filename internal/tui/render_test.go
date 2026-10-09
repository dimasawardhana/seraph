package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"seraph/internal/dashboard"
)

// item builds a task for layout tests. Only the fields row() reads are set.
func item(id, priority, title, holder string, mine bool) dashboard.Item {
	return dashboard.Item{
		ID:           id,
		Title:        title,
		Priority:     priority,
		ClaimHolder:  holder,
		ClaimedByMe:  mine,
		ClaimExpires: 0,
	}
}

// startOf returns the screen column at which needle begins, or -1. It measures
// the stripped row, so styling cannot hide a shift.
func startOf(line, needle string) int {
	plain := stripStyle(line)
	byteAt := strings.Index(plain, needle)
	if byteAt < 0 {
		return -1
	}
	// Count columns rather than bytes, so a multi-byte glyph earlier in the row
	// does not offset the answer.
	return lipgloss.Width(plain[:byteAt])
}

// A held row carries a holder and a free row does not, and the holder is the
// last column — so their titles must still begin at the same screen column, or
// the board cannot be read down. This is the alignment the board promises.
func TestHeldAndFreeRowsShareOneColumnGrid(t *testing.T) {
	m := &model{}
	// A five- and a six-digit id are wider than the nine-column id field, so they
	// exercise the cut: an uncut id pushes every later column right on its own row.
	free := item("TASK-101", "high", "Alpha title", "", false)
	long1 := item("TASK-10000", "high", "Beta title", "kiro-ee6cfe57", false)
	long2 := item("TASK-100000", "low", "Gamma title", "someone", false)
	held := item("TASK-102", "high", "Delta title", "kiro-ee6cfe57", false)
	mine := item("TASK-103", "high", "Epsilon title", "me", true)

	rows := []dashboard.Item{free, long1, long2, held, mine}
	for _, width := range []int{68, 80, 100, 120, 160, 240} {
		starts := make([]int, len(rows))
		for i, it := range rows {
			line := m.row(it, false, width)
			starts[i] = startOf(line, string([]rune(it.Title)[:5]))
		}
		for i := 1; i < len(starts); i++ {
			if starts[i] != starts[0] {
				t.Errorf("width %d: row %d starts its title at column %d, row 0 at %d — "+
					"the columns do not line up", width, i, starts[i], starts[0])
			}
		}
	}
}

// The id column is fixed at nine columns whatever the id itself measures, cut or not, so
// the column after it — the priority — must land at the same place on every row.
func TestPriorityStartsAtOneColumn(t *testing.T) {
	m := &model{}
	rows := []dashboard.Item{
		item("TASK-1", "urgent", "a", "", false),
		item("TASK-121", "high", "b", "", false),
		item("TASK-999", "medium", "c", "", false),
		item("TASK-1000", "low", "d", "", false),
		item("TASK-10000", "urgent", "e", "", false),
		item("TASK-100000", "high", "f", "", false),
	}
	for _, width := range []int{68, 80, 120, 200} {
		want := -1
		for _, it := range rows {
			line := m.row(it, false, width)
			// A cut id shows its first eight characters and an ellipsis, so the
			// needle is the first seven — present whether or not the id was cut.
			needle := it.ID
			if r := []rune(needle); len(r) > 7 {
				needle = string(r[:7])
			}
			got := startOf(line, needle)
			if got < 0 {
				t.Fatalf("width %d: id %q missing from %q", width, it.ID, stripStyle(line))
			}
			if want == -1 {
				want = got
				continue
			}
			if got != want {
				t.Errorf("width %d: id %q starts at column %d, want %d", width, it.ID, got, want)
			}
		}
	}
}

// Nothing may exceed the terminal width. A wrapped line becomes two rows in a
// panel that has already laid out its height, which pushes the board off the
// top of the screen.
func TestNoRenderedLineExceedsTheTerminalWidth(t *testing.T) {
	m := &model{}
	long := strings.Repeat("very long task title that keeps going and going ", 6)
	tasks := []dashboard.Item{
		item("TASK-101", "high", long, "kiro-ee6cfe57-ec4c-4638-a49b-9533d826fb42", false),
		item("TASK-102", "urgent", long, "someone-with-a-long-name", false),
		item("TASK-103", "medium", long, "", false),
		item("TASK-104", "low", "short", "me", true),
		item("TASK-10000", "high", long, "kiro-ee6cfe57", false),
		item("TASK-100000", "urgent", long, "someone-with-a-long-name", false),
	}

	for width := 40; width <= 240; width += 2 {
		for _, it := range tasks {
			for _, sel := range []bool{false, true} {
				line := m.row(it, sel, width)
				if got := lipgloss.Width(line); got > width {
					t.Errorf("width %d: row renders %d columns — it wraps. %q",
						width, got, stripStyle(line))
					break
				}
			}
		}
	}
}

// The frame is what actually reaches the terminal, so every line it emits must end with a
// carriage return as well as a newline. Run holds the terminal in raw mode, which disables
// the ONLCR translation that would supply the return — without it each line starts wherever
// the previous one stopped, and the board walks off the right edge.
func TestEveryFrameLineReturnsTheCarriage(t *testing.T) {
	long := strings.Repeat("wide title ", 20)
	m := &model{view: dashboard.View{Columns: []dashboard.Column{{
		Status: "backlog", Label: "backlog",
		Items: []dashboard.Item{
			item("TASK-101", "high", long, "someone", false),
			item("TASK-102", "low", long, "", false),
		},
	}}}}

	for _, width := range []int{60, 80, 120} {
		frame := m.frame(width, 20)
		split := strings.Split(frame, "\n")
		for i, line := range split {
			if i == len(split)-1 {
				break // nothing follows the final newline
			}
			if !strings.HasSuffix(line, "\r") {
				t.Errorf("width %d: line %d ends without \\r — it will not return to column 0",
					width, i)
			}
		}
	}
}

// A frame may not paint a line wider than the terminal. Those are the lines that wrap.
func TestNoFrameLineExceedsTheWidth(t *testing.T) {
	long := strings.Repeat("wide title ", 20)
	view := dashboard.View{Columns: []dashboard.Column{{
		Status: "backlog", Label: "backlog",
		Items: []dashboard.Item{
			item("TASK-101", "high", long, "someone-with-a-very-long-name", false),
			item("TASK-102", "low", long, "", false),
		},
	}}}

	for _, width := range []int{50, 60, 80, 120, 200} {
		for _, withCompose := range []bool{false, true} {
			m := &model{view: view}
			if withCompose {
				m.compose = &compose{fields: []string{strings.Repeat("t", 200), "", ""}}
			}
			frame := m.frame(width, 24)
			for i, line := range strings.Split(frame, "\n") {
				// clearEOL and the cursor sequences carry no width; strip first.
				plain := stripStyle(strings.ReplaceAll(line, "\r", ""))
				if got := lipgloss.Width(plain); got > width {
					t.Errorf("width %d compose=%v: line %d is %d columns — it wraps: %q",
						width, withCompose, i, got, plain)
				}
			}
		}
	}
}

// The compose panel is three rows, and each one is clamped: an answer longer than the
// terminal must not wrap, or the panel loses the rows the board needs.
func TestComposeLinesFitTheWidth(t *testing.T) {
	c := &compose{fields: []string{strings.Repeat("t", 200), strings.Repeat("g", 200), ""}}
	for _, width := range []int{30, 50, 80} {
		for i, line := range c.lines(width) {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: compose line %d is %d columns — it wraps", width, i, got)
			}
		}
	}
}
