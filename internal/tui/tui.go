package tui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"seraph/internal/board"
	"seraph/internal/dashboard"
	"seraph/internal/repo"
)

const (
	altEnter = "\x1b[?1049h"
	altExit  = "\x1b[?1049l"
	hideCurs = "\x1b[?25l"
	showCurs = "\x1b[?25h"
	home     = "\x1b[H"
	clearEOL = "\x1b[K"
	clearScr = "\x1b[2J"
	crlf     = "\r\n"
)

type model struct {
	board   *dashboard.Board
	view    dashboard.View
	col     int
	row     int
	detail  bool
	compose *compose
	message string
	drawn   int
}

func Run(root repo.Root) error {
	b, err := dashboard.Open(root)
	if err != nil {
		return err
	}
	defer b.Close()

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("seraph board needs a terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(fd, state)

	fmt.Print(altEnter + hideCurs)
	defer fmt.Print(showCurs + altExit)

	resized := make(chan os.Signal, 1)
	signal.Notify(resized, syscall.SIGWINCH)

	keys := make(chan string, 8)
	go readKeys(os.Stdin, keys)

	m := &model{board: b}
	if err := m.reload(); err != nil {
		return err
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		m.draw()
		select {
		case k := <-keys:
			if quit := m.handle(k); quit {
				return nil
			}
		case <-tick.C:
			m.message = ""
			_ = m.reload()
		case <-resized:
			_ = m.reload()
		}
	}
}
func (m *model) reload() error {
	view, err := m.board.View()
	if err != nil {
		return err
	}
	m.view = view
	m.clamp()
	return nil
}

// clamp keeps the cursor inside the column grid after every reload: a task that
// moved status, or a column that emptied, must not leave the cursor nowhere.
func (m *model) clamp() {
	if m.col < 0 {
		m.col = 0
	}
	if m.col >= len(m.view.Columns) {
		m.col = len(m.view.Columns) - 1
	}
	if m.col < 0 {
		m.col = 0
		return
	}
	if m.row >= len(m.view.Columns[m.col].Items) {
		m.row = len(m.view.Columns[m.col].Items) - 1
	}
	if m.row < 0 {
		m.row = 0
	}
}

func (m *model) current() (dashboard.Item, bool) {
	if m.col < 0 || m.col >= len(m.view.Columns) {
		return dashboard.Item{}, false
	}
	items := m.view.Columns[m.col].Items
	if m.row < 0 || m.row >= len(items) {
		return dashboard.Item{}, false
	}
	return items[m.row], true
}

func (m *model) handle(k string) (quit bool) {
	if m.compose != nil {
		return m.handlePrompt(k)
	}

	if m.detail {
		if k == "esc" || k == "enter" {
			m.detail = false
		}
		return false
	}

	switch k {
	case "q", "ctrl+c":
		return true
	case "j", "down":
		m.row++
	case "k", "up":
		m.row--
	case "h", "left":
		m.col--
	case "l", "right":
		m.col++
	}
	m.clamp()

	switch k {
	case "c":
		m.act(func(id string) error { return m.board.Claim(id) }, "Claimed")
	case "u":
		m.act(func(id string) error { return m.board.Release(id) }, "Released")
	case "s":
		m.act(func(id string) error { return m.board.SetStatus(id, "in_progress") }, "Started")
	case "r":
		m.act(func(id string) error { return m.board.SetStatus(id, "review") }, "Reviewing")
	case "d":
		m.act(func(id string) error { return m.board.SetStatus(id, "done") }, "Completed")
	case "n":
		m.startCompose()
	case "enter", " ":
		if _, ok := m.current(); ok {
			m.detail = true
		}
	}
	return false
}

func (m *model) act(fn func(id string) error, done string) {
	item, ok := m.current()
	if !ok {
		return
	}
	if err := fn(item.ID); err != nil {
		m.message = errText(err)
	} else {
		m.message = done + " " + item.ID
	}
	_ = m.reload()
}

func (m *model) handlePrompt(k string) bool {
	c := m.compose
	switch k {
	case "esc", "ctrl+c":
		m.compose = nil
	case "enter":
		if !c.advance() {
			return false
		}
		title, goal, acceptance, ok := c.submit()
		m.compose = nil
		if !ok {
			return false
		}
		if _, err := m.board.Create(title, goal, "", acceptance, "", ""); err != nil {
			m.message = errText(err)
		} else {
			m.message = "Created " + title
		}
		_ = m.reload()
	case "backspace":
		if c.fields[c.at] == "" {
			c.retreat()
		} else {
			c.backspace()
		}
	case "tab":
		if !c.advance() {
			m.compose = nil
			_, _, _, _ = c.submit()
		}
	case "space":
		c.typeRune(" ")
	default:
		c.typeRune(k)
	}
	return false
}

func errText(err error) string {
	if typed, ok := board.AsError(err); ok {
		return typed.Reason + " — " + typed.Message
	}
	return err.Error()
}

// draw paints the panel once.
func (m *model) draw() {
	width, height := size()
	fmt.Print(m.frame(width, height))
}

// frame builds one repaint. It moves the cursor home and rewrites each line in place
// rather than clearing the screen, because a full clear every second flickers hard enough
// to be unpleasant on a second monitor. Only the first paint clears.
//
// Every line ends in \r\n, never a bare \n. Run puts the terminal in raw mode, which turns
// off the ONLCR translation that would otherwise return the carriage for us — so a bare \n
// leaves the cursor in the column it was already in, and each line begins further right
// than the one above it until the board walks off the right edge and wraps.
func (m *model) frame(width, height int) string {
	lines, cursor := m.lines(width, height)

	compose := []string{}
	if m.compose != nil {
		compose = m.compose.lines(width)
	}
	message := ""
	if m.message != "" {
		message = "  " + m.message
	}

	pad := height - 3 - len(compose)
	if message != "" {
		pad--
	}
	if pad < 1 {
		pad = 1
	}

	start := 0
	if cursor >= pad && len(lines) > pad {
		start = cursor - pad + 1
		if start > len(lines)-pad {
			start = len(lines) - pad
		}
	}
	shown := lines[start:]
	if len(shown) > pad {
		shown = shown[:pad]
	}

	var b strings.Builder
	if m.drawn == 0 {
		b.WriteString(clearScr)
	}
	b.WriteString(home)

	for i := range pad {
		text := ""
		if i < len(shown) {
			text = shown[i]
		}
		b.WriteString(text + clearEOL + crlf)
	}

	for _, line := range compose {
		b.WriteString(line + clearEOL + crlf)
	}
	b.WriteString(message + clearEOL + crlf)
	b.WriteString(faint.Render(keymap(width)) + clearEOL)

	m.drawn++
	return b.String()
}

// lines renders the board as kanban columns: one per status, laid out side by
// side, each column a stack of cards. The column the cursor sits in is the
// active one; the others render faint. When the detail overlay is open it
// replaces the board view.
func (m *model) lines(width, height int) ([]string, int) {
	if m.detail {
		if item, ok := m.current(); ok {
			return detailLines(item, width), 0
		}
		m.detail = false
	}

	cols := m.view.Columns
	if len(cols) == 0 {
		return []string{"  " + faint.Render("The board is empty. Press n to add a task.")}, 0
	}

	each := width / len(cols)
	if each < 14 {
		each = 14
	}

	deepest := 0
	for _, c := range cols {
		if len(c.Items) > deepest {
			deepest = len(c.Items)
		}
	}

	var out []string
	cursor := 0
	out = append(out, m.header(width, each)...)
	cursor += len(out)
	for slot := range deepest {
		line := ""
		for ci, c := range cols {
			card := ""
			if slot < len(c.Items) {
				card = m.card(c.Items[slot], ci == m.col && slot == m.row, ci == m.col, each)
			}
			line += padRight(card, each)
		}
		out = append(out, line)
		if m.col < len(cols) && slot == m.row {
			cursor = len(out) - 1
		}
	}
	return out, cursor
}

// header renders one column plate per status, with counts, all on one line.
func (m *model) header(width, each int) []string {
	line := ""
	for _, c := range m.view.Columns {
		p := plate.Render(strings.ToUpper(c.Label)) +
			faint.Render(fmt.Sprintf(" %d", len(c.Items)))
		line += padRight(" "+p, each)
	}
	return []string{line}
}

func padRight(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		if lipgloss.Width(string(runes)) <= width-1 {
			return string(runes) + "…"
		}
	}
	return ""
}

// keymap shortens rather than overflowing. A terminal wraps a too-long line into the
// next row, which pushes the board off screen — the one thing a fixed-height panel
// cannot survive.
func keymap(width int) string {
	full := "  j k move · c claim · s start · r review · d done · u release · n new · q quit"
	if width >= lipgloss.Width(full) {
		return full
	}
	short := "  j k move · c claim · d done · n new · q quit"
	if width >= lipgloss.Width(short) {
		return short
	}
	return "  j k · c · d · n · q"
}

func pad(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return "  " + left + strings.Repeat(" ", gap) + right
}

func size() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 100, 30
	}
	return w, h
}

func readKeys(f *os.File, out chan<- string) {
	buf := make([]byte, 8)
	for {
		n, err := f.Read(buf)
		if err != nil || n == 0 {
			close(out)
			return
		}
		out <- decode(buf[:n])
	}
}

func decode(b []byte) string {
	s := string(b)
	switch s {
	case "\x1b[A", "\x1bOA":
		return "up"
	case "\x1b[B", "\x1bOB":
		return "down"
	case "\x1b[C", "\x1bOC":
		return "right"
	case "\x1b[D", "\x1bOD":
		return "left"
	case "\x1b":
		return "esc"
	case "\x03":
		return "ctrl+c"
	case "\r", "\n":
		return "enter"
	case "\x7f", "\b":
		return "backspace"
	case " ":
		return "space"
	}
	if len(b) == 1 && b[0] >= 0x20 && b[0] < 0x7f {
		return s
	}
	return ""
}
