package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"seraph/internal/repo"
)

const ServerName = "seraph"

// hooks holds the most recent install's gate report, so the CLI can present file changes
// and gate coverage together without threading a second return value through callers.
var hooks []HookReport

// Hooks returns the gate report from the most recent Install.
func Hooks() []HookReport { return hooks }

var targets = []string{".mcp.json", "AGENTS.md", "CLAUDE.md"}

type Change struct {
	Path   string
	Action string
	Detail string
	Before string
	After  string
}

func (c Change) Changed() bool { return c.Before != c.After }

func Install(root repo.Root, dryRun bool) ([]Change, error) {
	var changes []Change

	for _, name := range targets {
		path := filepath.Join(root.Path, name)
		existing, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		var updated []byte
		var detail string
		switch name {
		case ".mcp.json":
			updated, detail, err = mergeMCPConfig(path, existing)
		default:
			var merged string
			merged, err = mergeBlock(string(existing), Rules)
			updated = []byte(merged)
			detail = fmt.Sprintf("Rules block, %d bytes", len(Rules))
		}
		if err != nil {
			return nil, err
		}

		change := Change{
			Path:   path,
			Before: string(existing),
			After:  string(updated),
			Detail: detail,
		}
		switch {
		case len(existing) == 0:
			change.Action = "created"
		case !change.Changed():
			change.Action = "unchanged"
		default:
			change.Action = "updated"
		}
		changes = append(changes, change)

		if change.Action != "unchanged" && !dryRun {
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				return nil, fmt.Errorf("write %s: %w", path, err)
			}
		}
	}

	ignore := filepath.Join(root.StatePath(), ".gitignore")
	changes = append(changes, writeIfChanged(ignore, []byte(StateIgnore), dryRun,
		fmt.Sprintf("ignores state.db*, keeps snapshots committable (%d bytes)", len(StateIgnore))))

	reports, err := InstallHooks(root.Path, SelfPath(), dryRun)
	if err != nil {
		return nil, err
	}
	hooks = reports

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func writeIfChanged(path string, content []byte, dryRun bool, detail string) Change {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == string(content) {
		return Change{Path: path, Action: "unchanged", Detail: detail,
			Before: string(existing), After: string(content)}
	}
	action := "updated"
	if os.IsNotExist(err) {
		action = "created"
	}

	change := Change{Path: path, Action: action, Detail: detail,
		Before: string(existing), After: string(content)}

	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			change.Detail = "FAILED: " + err.Error()
			return change
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			change.Detail = "FAILED: " + err.Error()
		}
	}
	return change
}

func mergeMCPConfig(path string, existing []byte) ([]byte, string, error) {
	entry := map[string]any{
		"command": ServerName,
		"args":    []string{},
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return nil, "", err
	}

	doc := map[string]json.RawMessage{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &doc); err != nil {
			return nil, "", fmt.Errorf("refusing to rewrite %s: it exists but is not valid JSON (%v)", path, err)
		}
	}

	servers := map[string]json.RawMessage{}
	if raw, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, "", fmt.Errorf("refusing to rewrite %s: mcpServers is not an object (%v)", path, err)
		}
	}
	if current, ok := servers[ServerName]; ok && jsonEqual(current, entryJSON) {
		return existing, "Seraph already registered; other servers untouched", nil
	}
	servers[ServerName] = entryJSON

	merged, err := json.Marshal(servers)
	if err != nil {
		return nil, "", err
	}
	doc["mcpServers"] = merged

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	detail := fmt.Sprintf("registered %q; %d other server(s) preserved", ServerName, len(servers)-1)
	return append(out, '\n'), detail, nil
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func Report(w io.Writer, changes []Change, dryRun bool) {
	if dryRun {
		fmt.Fprintln(w, "seraph install --dry-run: nothing was written")
	} else {
		fmt.Fprintln(w, "seraph install")
	}
	for _, c := range changes {
		fmt.Fprintf(w, "\n  %-9s %s\n", c.Action, c.Path)
		if c.Detail != "" {
			fmt.Fprintf(w, "            %s\n", c.Detail)
		}
		if c.Changed() {
			removed, added := lineDiff(c.Before, c.After)
			for _, line := range removed {
				fmt.Fprintf(w, "            - %s\n", line)
			}
			for _, line := range added {
				fmt.Fprintf(w, "            + %s\n", line)
			}
		}
	}
	fmt.Fprintln(w, "\nThe Rules block is advisory. The server enforces the claim rule itself, so a")
	fmt.Fprintln(w, "harness that never reads these files still cannot complete unclaimed work.")
	fmt.Fprintln(w, "\nHarnesses whose hooks cannot block are reported below with the gate that does work.")
	fmt.Fprintln(w, "Config files holding live credentials are never rewritten — only extended.")
	fmt.Fprintln(w, "`seraph doctor` reports which harnesses are registered and which are not.")
}
