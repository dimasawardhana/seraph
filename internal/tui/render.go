package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"seraph/internal/dashboard"
)

var (
	plate    = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	faint    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	txt      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	doneTxt  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	clear    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	held     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	caution  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	mineStyl = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	cursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	chosen   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
)

const cautionWindow = 5 * time.Minute

// aspect mirrors the browser panel, so a task reads the same in both rooms: the aspect
// first, then the work, then who holds it and until when.
func aspect(i dashboard.Item) string {
	switch {
	case i.ClaimedByMe:
		return "mine"
	case i.ClaimHolder == "":
		return "free"
	case time.Until(time.Unix(i.ClaimExpires, 0)) < cautionWindow:
		return "caution"
	default:
		return "held"
	}
}

func dot(a string) string {
	switch a {
	case "mine":
		return mineStyl.Render("●")
	case "held":
		return held.Render("●")
	case "caution":
		return caution.Render("●")
	default:
		return clear.Render("○")
	}
}

func remaining(expires int64) string {
	if expires == 0 {
		return ""
	}
	left := time.Until(time.Unix(expires, 0))
	switch {
	case left <= 0:
		return " · lapsed"
	case left < time.Minute:
		return fmt.Sprintf(" · %ds", int(left.Seconds()))
	case left < time.Hour:
		return fmt.Sprintf(" · %dm", int(left.Minutes()))
	default:
		return fmt.Sprintf(" · %dh%02dm", int(left.Hours()), int(left.Minutes())%60)
	}
}

func holderText(i dashboard.Item) string {
	switch {
	case i.ClaimedByMe:
		return mineStyl.Render("you") + faint.Render(remaining(i.ClaimExpires))
	case i.ClaimHolder != "":
		return txt.Render(i.ClaimHolder) + faint.Render(remaining(i.ClaimExpires))
	default:
		return clear.Render("clear")
	}
}

func idText(i dashboard.Item) string {
	switch i.Priority {
	case "urgent":
		return held.Render(i.ID)
	case "high":
		return caution.Render(i.ID)
	default:
		return faint.Render(i.ID)
	}
}

// columns sizes the row from the real terminal width, so a narrow window drops a column
// rather than letting one wrap.
func columns(width int) (id, prio, title, holder int) {
	id, prio, holder = 9, 8, 18
	title = width - id - prio - holder - 12
	if title < 14 {
		title = 14
	}
	if width < 68 {
		prio = 0
	}
	if width < 46 {
		holder = 0
	}
	return
}

func (m *model) row(item dashboard.Item, isSelected bool, width int) string {
	idw, priow, titlew, holderw := columns(width)
	a := aspect(item)

	marker := "  "
	if isSelected {
		marker = cursor.Render("▸ ")
	}

	id := ""
	if idw > 0 {
		id = padRight(idText(item), idw)
	}

	// Pad the rendered run, not the source word: substituting a glyph for "urgent"
	// changes the display width, and padding the word leaves the column ragged.
	prio := ""
	if priow > 0 {
		switch item.Priority {
		case "urgent":
			prio = padRight(held.Render("!"), priow)
		case "high":
			prio = padRight(caution.Render("↑"), priow)
		default:
			prio = padRight(faint.Render(item.Priority), priow)
		}
	}

	// Truncate the plain string, then style it. Cutting runes inside a styled run would
	// split an escape sequence and spill colour across the rest of the panel.
	plain := cut(item.Title, titlew)
	title := ""
	switch {
	case isSelected:
		// Three signals, because a background block is the first thing to vanish on a
		// terminal without colour, or when the output goes through a pipe.
		title = padRight(chosen.Render(plain), titlew)
	case item.Status == "done":
		title = padRight(doneTxt.Render(plain), titlew)
	default:
		title = padRight(txt.Render(plain), titlew)
	}

	holder := ""
	if holderw > 0 {
		holder = padRight(cut(stripStyle(holderText(item)), holderw), holderw)
	}

	return strings.TrimRight(
		fmt.Sprintf("%s %s %s %s %s %s", marker, dot(a), id, prio, title, holder), " ")
}

// cut shortens to a display width. It measures with lipgloss rather than counting runes,
// so CJK and emoji do not overflow the column.
func cut(s string, width int) string {
	if width <= 0 {
		return ""
	}
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

// stripStyle removes escape sequences so a styled run can be measured as plain text.
func stripStyle(s string) string {
	var b strings.Builder
	plain := true
	for _, r := range s {
		if plain {
			if r == 0x1b {
				plain = false
				continue
			}
			b.WriteRune(r)
			continue
		}
		// A CSI sequence ends on a letter; anything else keeps us inside it.
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			plain = true
		}
	}
	return b.String()
}
