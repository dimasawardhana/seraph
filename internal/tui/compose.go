package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// A compose is three answers, asked in the order that keeps them useful: what it is
// called, what it is for, and how anyone will know it was done. The terminal asks for all
// three because the server requires all three — a one-line prompt cannot satisfy the same
// contract the browser form does.
type compose struct {
	fields     []string
	at         int
	title      string
	goal       string
	acceptance string
}

var prompts = []string{"title", "goal", "done when"}

func (m *model) startCompose() {
	m.compose = &compose{fields: []string{"", "", ""}}
}

func (c *compose) current() string { return prompts[c.at] }
func (c *compose) value() string   { return c.fields[c.at] }
func (c *compose) full() bool      { return c.at >= len(prompts)-1 }

func (c *compose) typeRune(k string) {
	if len([]rune(k)) == 1 {
		r := []rune(c.fields[c.at])
		c.fields[c.at] = string(append(r, []rune(k)...))
	}
}

func (c *compose) backspace() {
	r := []rune(c.fields[c.at])
	if len(r) > 0 {
		c.fields[c.at] = string(r[:len(r)-1])
	}
}

// advance moves to the next prompt, returning true when the last one is reached and the
// answers are ready to submit.
func (c *compose) advance() bool {
	if c.at < len(prompts)-1 {
		c.at++
		return false
	}
	return true
}

func (c *compose) retreat() {
	if c.at > 0 {
		c.at--
	}
}

// lines renders each prompt as one row, clamped to the terminal width. A typed answer that
// runs past the edge would wrap exactly as a board row would, and the panel has no height
// left to give — so the field scrolls under the cursor instead, keeping the end of what is
// being typed visible. The clamp is unconditional: when the terminal is narrower than the
// prompt itself, the label is dropped and the value alone is shown, still clamped.
func (c *compose) lines(width int) []string {
	out := make([]string, 0, len(prompts))
	for i, p := range prompts {
		marker := "  "
		if i == c.at {
			marker = cursor.Render("▸ ")
		}
		label := p + ": "
		value := c.fields[i]
		empty := value == ""
		if empty && i != c.at {
			value = "—"
		}
		room := width - lipgloss.Width(marker) - lipgloss.Width(label)
		if room < 0 {
			room = 0
		}
		runes := []rune(value)
		if len(runes) > room {
			value = string(runes[len(runes)-room:])
		}
		style := txt
		if empty {
			style = faint
		}
		line := marker + style.Render(label+value)
		if got := lipgloss.Width(line); got > width {
			keep := width
			if keep > len(runes) {
				keep = len(runes)
			}
			if keep < 0 {
				keep = 0
			}
			line = style.Render(string(runes[len(runes)-keep:]))
		}
		out = append(out, line)
	}
	return out
}

func (c *compose) submit() (title, goal, acceptance string, ok bool) {
	title = strings.TrimSpace(c.fields[0])
	goal = strings.TrimSpace(c.fields[1])
	acceptance = strings.TrimSpace(c.fields[2])
	return title, goal, acceptance, title != ""
}
