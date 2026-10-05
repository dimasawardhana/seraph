package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"seraph/internal/installer"
	"seraph/internal/repo"
)

type harness struct {
	Name   string
	Path   string
	Key    string
	Format string
}

func userHarnesses() []harness {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	join := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	return []harness{
		{"opencode", join(".config", "opencode", "opencode.json"), "mcp", "json"},
		{"Claude Code", join(".claude.json"), "mcpServers", "json"},
		{"omp", join(".omp", "agent", "mcp.json"), "mcpServers", "json"},
		{"Cursor", join(".cursor", "mcp.json"), "mcpServers", "json"},
		{"Cline", join(".cline", "mcp.json"), "mcpServers", "json"},
		{"Kiro", join(".kiro", "settings", "mcp.json"), "mcpServers", "json"},
		{"Gemini CLI", join(".gemini", "settings.json"), "mcpServers", "json"},
		{"Antigravity", join(".gemini", "antigravity", "mcp_config.json"), "mcpServers", "json"},
		{"Copilot CLI", join(".copilot", "mcp-config.json"), "mcpServers", "json"},
		{"Zed", join(".config", "zed", "settings.json"), "context_servers", "json"},
		{"openclaw", join(".openclaw", "openclaw.json"), "mcp.servers", "json"},
		{"hermes", join(".hermes", "config.yaml"), "mcp_servers", "yaml"},
	}
}

func registered(h harness) bool {
	raw, err := os.ReadFile(h.Path)
	if err != nil {
		return false
	}
	if h.Format == "yaml" {
		return strings.Contains(string(raw), installer.ServerName+":")
	}

	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	container, ok := doc[h.Key]
	if !ok {
		return false
	}
	switch typed := container.(type) {
	case map[string]any:
		_, found := typed[installer.ServerName]
		return found
	case []any:
		for _, entry := range typed {
			if m, ok := entry.(map[string]any); ok {
				if name, ok := m["command"].(string); ok && name == installer.ServerName {
					return true
				}
			}
		}
	}
	return false
}

func reportHarnesses(w io.Writer, r repo.Root) {
	fmt.Fprintln(w, "\nharnesses")
	fmt.Fprintln(w, "  project scope — written by `seraph install`")

	project := []struct {
		name, path string
		registered bool
	}{
		{"MCP config", filepath.Join(r.Path, ".mcp.json"), hasSeraphInMCP(filepath.Join(r.Path, ".mcp.json"))},
		{"Rules (AGENTS.md)", filepath.Join(r.Path, "AGENTS.md"), hasRules(filepath.Join(r.Path, "AGENTS.md"))},
		{"Rules (CLAUDE.md)", filepath.Join(r.Path, "CLAUDE.md"), hasRules(filepath.Join(r.Path, "CLAUDE.md"))},
	}
	for _, p := range project {
		line(w, p.name, p.path, p.registered, "")
	}

	fmt.Fprintln(w, "  user scope — reported, never edited (several hold live credentials)")
	for _, h := range userHarnesses() {
		line(w, h.Name, h.Path, registered(h), h.Key)
	}
}

func line(w io.Writer, name, path string, isRegistered bool, key string) {
	state := "present, no seraph entry"
	switch {
	case isRegistered:
		state = "registered"
	case !exists(path):
		state = "no config"
	}
	fmt.Fprintf(w, "    %-10s %-26s %-30s %s\n", state, name, shorten(path), "key "+key)
}

func shorten(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || !strings.HasPrefix(path, home+string(filepath.Separator)) {
		return path
	}
	return "~" + path[len(home):]
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasSeraphInMCP(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	doc := struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	_, ok := doc.MCPServers[installer.ServerName]
	return ok
}

func hasRules(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(raw)
	return strings.Contains(content, installer.StartMarker) && strings.Contains(content, installer.EndMarker)
}
