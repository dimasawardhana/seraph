package vocab

import "strings"

type Option struct {
	Value   string
	Label   string
	Default bool
}

var Statuses = []Option{
	{Value: "in_progress", Label: "In Progress"},
	{Value: "review", Label: "Review"},
	{Value: "todo", Label: "To Do"},
	{Value: "backlog", Label: "Backlog", Default: true},
	{Value: "done", Label: "Done"},
}

var Priorities = []Option{
	{Value: "urgent"},
	{Value: "high"},
	{Value: "medium", Default: true},
	{Value: "low"},
}

var Triages = []Option{
	{Value: "needs-triage"},
	{Value: "needs-info"},
	{Value: "ready-for-agent"},
	{Value: "ready-for-human"},
	{Value: "wontfix"},
}

func Values(options []Option) []string {
	out := make([]string, len(options))
	for i, o := range options {
		out[i] = o.Value
	}
	return out
}

func Has(options []Option, value string) bool {
	for _, o := range options {
		if o.Value == value {
			return true
		}
	}
	return false
}

func Order(options []Option, value string) int {
	for i, o := range options {
		if o.Value == value {
			return i
		}
	}
	return len(options)
}

func Label(options []Option, value string) string {
	for _, o := range options {
		if o.Value == value && o.Label != "" {
			return o.Label
		}
	}
	return value
}

func Default(options []Option) string {
	for _, o := range options {
		if o.Default {
			return o.Value
		}
	}
	return ""
}

func Quoted(options []Option) string {
	values := Values(options)
	for i, v := range values {
		values[i] = "'" + v + "'"
	}
	return strings.Join(values, ", ")
}
