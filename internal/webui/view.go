package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"strings"
	"time"

	"seraph/internal/dashboard"
)

func funcs() template.FuncMap {
	return template.FuncMap{
		"since":  since,
		"ttl":    remaining,
		"title":  clip,
		"aspect": aspect,
		"held":   heldCount,
		"hash":   hashView,
	}
}

func since(unix int64) string {
	if unix == 0 {
		return ""
	}
	d := time.Since(time.Unix(unix, 0)).Truncate(time.Second)
	if d < time.Minute {
		return d.String() + " ago"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// remaining renders a claim's clearing time, and its state without a legend:
// minutes still to run, or that it has already lapsed.
func remaining(expires int64) string {
	if expires == 0 {
		return ""
	}
	left := time.Until(time.Unix(expires, 0))
	if left <= 0 {
		return "· lapsed"
	}
	if left < time.Minute {
		return fmt.Sprintf("· %ds", int(left.Seconds()))
	}
	return fmt.Sprintf("· %dm", int(left.Minutes()))
}

// aspect is the signal aspect of a task: whether the block is clear, yours, or
// someone else's. Caution is a claim about to clear, which is the one state where
// acting late changes the outcome.
func aspect(i dashboard.Item) string {
	switch {
	case i.ClaimedByMe:
		return "mine"
	case i.ClaimHolder == "":
		return "free"
	case time.Until(time.Unix(i.ClaimExpires, 0)) < 5*time.Minute:
		return "caution"
	default:
		return "held"
	}
}

func heldCount(v dashboard.View) int { return dashboard.HumanHolders(v) }

func clip(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 140 {
		return string([]rune(s)[:139]) + "…"
	}
	return s
}

// hashView fingerprints the rendered board so the client can tell an actual change
// from an idle poll, and leave the page alone when nothing moved.
func hashView(v dashboard.View) string {
	h := sha256.New()
	for _, c := range v.Columns {
		for _, i := range c.Items {
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%t\x00",
				i.ID, i.Title, i.Goal, i.Acceptance, i.Status, i.Triage, i.Priority,
				i.ClaimHolder, i.ClaimExpires, i.ClaimedByMe)
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}
