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
)

type model struct {
	board    *dashboard.Board
	view     dashboard.View
	flat     []dashboard.Item
	selected int
	compose  *compose
	message  string
	drawn    int
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
	m.flat = m.flat[:0]
	for _, c := range view.Columns {
		m.flat = append(m.flat, c.Items...)
	}
	if m.selected >= len(m.flat) {
		m.selected = len(m.flat) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	return nil
}

func (m *model) current() (dashboard.Item, bool) {
	if m.selected < 0 || m.selected >= len(m.flat) {
		return dashboard.Item{}, false
	}
	return m.flat[m.selected], true
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

func (m *model) handle(k string) (quit bool) {
	if m.compose != nil {
		return m.handlePrompt(k)
	}

	switch k {
	case "q", "ctrl+c":
		return true
	case "j", "down":
		if m.selected < len(m.flat)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "g":
		m.selected = 0
	case "G":
		m.selected = len(m.flat) - 1
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
	}
	return false
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

// draw repaints the panel. It moves the cursor home and rewrites each line in place
// rather than clearing the screen, because a full clear every second flickers hard enough
// to be unpleasant on a second monitor. Only the first paint clears.
func (m *model) draw() {
	width, height := size()
	lines, cursor := m.lines(width)
	pad := height - 5
	if m.compose != nil {
		pad -= 3
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

	for i := 0; i < pad; i++ {
		text := ""
		if i < len(shown) {
			text = shown[i]
		}
		b.WriteString(text + clearEOL + "\n")
	}

	bottom := ""
	if m.compose != nil {
		bottom = m.compose.line()
	} else if m.message != "" {
		bottom = "  " + m.message
	}
	b.WriteString(bottom + clearEOL + "\n")
	b.WriteString(faint.Render(keymap(width)) + clearEOL)

	m.drawn++
	fmt.Print(b.String())
}

func (m *model) lines(width int) ([]string, int) {
	var out []string
	cursor := 0
	index := 0
	for _, c := range m.view.Columns {
		out = append(out, "",
			"  "+plate.Render(strings.ToUpper(c.Label))+" "+faint.Render(fmt.Sprintf("%d", len(c.Items))))
		for _, item := range c.Items {
			if index == m.selected {
				cursor = len(out)
			}
			out = append(out, m.row(item, index == m.selected, width))
			index++
		}
	}
	if len(out) == 0 {
		out = []string{"  " + faint.Render("The board is empty. Press n to add a task.")}
	}
	return out, cursor
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
