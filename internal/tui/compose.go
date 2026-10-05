package tui

import "strings"

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

// line renders the prompt plus what has been entered so far, so the earlier answers stay
// on screen and can be corrected with one keystroke rather than retyped.
func (c *compose) line() string {
	var b strings.Builder
	for i, p := range prompts {
		marker := "  "
		if i == c.at {
			marker = cursor.Render("▸ ")
		}
		b.WriteString(marker)
		if c.fields[i] == "" && i != c.at {
			b.WriteString(faint.Render(p + ": —"))
		} else if c.fields[i] == "" {
			b.WriteString(faint.Render(p + ": "))
		} else {
			b.WriteString(txt.Render(p + ": " + c.fields[i]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (c *compose) submit() (title, goal, acceptance string, ok bool) {
	title = strings.TrimSpace(c.fields[0])
	goal = strings.TrimSpace(c.fields[1])
	acceptance = strings.TrimSpace(c.fields[2])
	return title, goal, acceptance, title != ""
}
