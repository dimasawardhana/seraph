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

// idText styles an id that has already been cut to its column. Taking the shown text
// rather than the Item keeps the cut on the plain string, where it cannot land inside an
// escape sequence.
func idText(i dashboard.Item, shown string) string {
	switch i.Priority {
	case "urgent":
		return held.Render(shown)
	case "high":
		return caution.Render(shown)
	default:
		return faint.Render(shown)
	}
}

// cardChrome is everything in a card that is not text: the marker, the dot,
// and the spaces between the fields.
const cardChrome = 2 + 1 + 5

// card renders one task inside its column. An active column's cards carry the
// full aspect signal; a background column's render faint, so the eye goes to
// the column the cursor is in before it reads anything else.
func (m *model) card(item dashboard.Item, isSelected, active bool, width int) string {
	free := width - cardChrome
	if free < 8 {
		free = 8
	}
	idw, holderw := 9, 0
	if free >= 30 {
		holderw = 12
	}
	titlew := free - idw - holderw
	if titlew < 1 {
		titlew = 1
	}

	marker := "  "
	if isSelected {
		marker = cursor.Render("▸ ")
	}

	id := padRight(idText(item, cut(item.ID, idw)), idw)

	plain := cut(item.Title, titlew)
	title := ""
	switch {
	case isSelected:
		title = padRight(chosen.Render(plain), titlew)
	case item.Status == "done":
		title = padRight(doneTxt.Render(plain), titlew)
	case active:
		title = padRight(txt.Render(plain), titlew)
	default:
		title = padRight(faint.Render(plain), titlew)
	}

	holder := ""
	if holderw > 0 {
		holder = padRight(cut(stripStyle(holderText(item)), holderw), holderw)
	}

	return strings.TrimRight(
		fmt.Sprintf("%s %s %s %s %s", marker, dot(aspect(item)), id, title, holder), " ")
}

// detailLines renders the detail overlay: the full record of one task, since
// the board's own promise is that a task carries its goal and acceptance and
// the compact card cannot hold them.
func detailLines(item dashboard.Item, width int) []string {
	wrap := func(s string) []string {
		if s == "" {
			return nil
		}
		var out []string
		for _, para := range strings.Split(s, "\n") {
			for len(para) > width-4 {
				cutAt := width - 4
				if cutAt >= len(para) {
					cutAt = len(para) - 1
				}
				if sp := strings.LastIndex(para[:cutAt], " "); sp > 0 {
					cutAt = sp
				}
				out = append(out, "  "+para[:cutAt])
				para = para[cutAt:]
				para = strings.TrimLeft(para, " ")
			}
			out = append(out, "  "+para)
		}
		return out
	}

	head := "  " + plate.Render(strings.ToUpper(item.ID)) + "  " + txt.Render(item.Title)
	out := []string{head, ""}
	out = append(out, faint.Render("  GOAL"))
	out = append(out, wrap(item.Goal)...)
	if item.Acceptance != "" {
		out = append(out, faint.Render("  DONE WHEN"))
		out = append(out, wrap(item.Acceptance)...)
	}
	if item.Description != "" {
		out = append(out, faint.Render("  NOTES"))
		out = append(out, wrap(item.Description)...)
	}
	out = append(out, "")
	meta := fmt.Sprintf("  %s · %s", dashboard.StatusLabel(item.Status), item.Priority)
	if item.Triage != "" {
		meta += " · " + item.Triage
	}
	if item.ClaimHolder != "" {
		meta += " · " + stripStyle(holderText(item))
	}
	out = append(out, faint.Render(meta))
	out = append(out, "", "  "+faint.Render("esc back"))
	return out
}

// stripStyle removes escape sequences so a styled run can be measured as plain text.
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
