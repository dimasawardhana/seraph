package gate

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"strings"
)

// stdin is the union of the hook payload shapes across harnesses. They disagree on almost
// every field name, so each is accepted under the names the survey recorded and the first
// non-empty wins.
type payload struct {
	Tool       string `json:"tool_name"`
	ToolAlt    string `json:"toolName"`
	ToolThird  string `json:"tool"`
	Session    string `json:"session_id"`
	SessionAlt string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Input      any    `json:"tool_input"`
	InputAlt   any    `json:"toolArgs"`
	Params     any    `json:"params"`
	FilePath   string `json:"file_path"`
}

// Run is the `seraph gate` subcommand: the executable a harness's pre-tool hook invokes.
//
// It answers one question on stdout in the shape the named harness expects, and exits 2
// on a refusal, which every blocking harness treats as a denial regardless of whether it
// parsed the body.
func Run(args []string, in io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harness := fs.String("harness", "", "harness name, selecting the output contract")
	tool := fs.String("tool", "", "tool name, overriding the payload")
	session := fs.String("session", "", "session id, overriding the payload")
	dir := fs.String("dir", "", "working directory, overriding the payload")
	explain := fs.Bool("explain", false, "print the reason to stderr and exit 0")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var body payload
	if raw, err := io.ReadAll(in); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}

	root := first(*dir, body.Cwd, ".")
	// One identity, resolved the same way everywhere. The payload wins, because a harness
	// that sets it knows more than we do; SERAPH_SESSION_ID is the fallback for the ones
	// that do not, and it is the same variable the generated hooks pass through. Without
	// this, a hook and an MCP call in one session could disagree about who the session is.
	sessionID := first(*session, body.Session, body.SessionAlt, os.Getenv("SERAPH_SESSION_ID"))
	toolName := first(*tool, body.Tool, body.ToolAlt, body.ToolThird)

	// Seraph must never block the writing of its own snapshots, or a harness would
	// deadlock against a board it cannot refresh.
	if PathExempt(editedPath(body)) {
		emit(stdout, true, "", "", *harness)
		return 0
	}

	input := any(nil)
	for _, source := range []any{body.Input, body.InputAlt, body.Params} {
		if m, ok := source.(map[string]any); ok && len(m) > 0 {
			input = m
			break
		}
	}
	verdict := Evaluate(root, toolName, sessionID, input)
	if *explain {
		fmt.Fprintln(stderr, "seraph gate:", verdict.Reason)
		emit(stdout, verdict.Gate.Allow, verdict.Gate.Reason, verdict.Gate.Task, *harness)
		return 0
	}
	emit(stdout, verdict.Gate.Allow, verdict.Gate.Reason, verdict.Gate.Task, *harness)
	if !verdict.Gate.Allow {
		fmt.Fprintln(stderr, verdict.Gate.Reason)
		return 2
	}
	if verdict.Gate.Task != "" {
		fmt.Fprintf(stderr, "seraph: holding %s\n", verdict.Gate.Task)
	}
	return 0
}

// emit writes the decision in the shape the named harness expects. An unrecognised harness
// gets the Claude Code / Copilot CLI permissionDecision form, because every harness that
// can block accepts that key — and one that cannot will rely on the exit code regardless.
func emit(w io.Writer, allow bool, reason, task, harness string) {
	decision := "allow"
	if !allow {
		decision = "deny"
	}

	var payload any
	switch harness {
	case "omp":
		payload = map[string]any{"block": !allow, "reason": reason}
	case "openclaw":
		payload = map[string]any{"block": !allow, "blockReason": reason}
	case "cursor":
		payload = map[string]any{"permission": decision, "agent_message": reason}
	case "gemini":
		payload = map[string]any{"decision": decision, "reason": reason}
	case "hermes":
		payload = map[string]any{"decision": decision, "reason": reason, "message": reason}
	case "cline":
		payload = map[string]any{"skip": !allow, "reason": reason}
	default:
		payload = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       decision,
				"permissionDecisionReason": reason,
			},
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(w, `{"hookSpecificOutput":{"permissionDecision":"allow"}}`)
		return
	}
	fmt.Fprintln(w, string(encoded))
	if task != "" {
		fmt.Fprintf(os.Stderr, "seraph: holding %s\n", task)
	}
}

// editedPath finds the file a write tool is touching.
func editedPath(p payload) string {
	target := p.FilePath
	if target == "" {
		for _, source := range []any{p.Input, p.InputAlt, p.Params} {
			m, ok := source.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"file_path", "filePath", "path", "file", "target_file", "notebook_path"} {
				if v, ok := m[key].(string); ok && v != "" {
					target = v
					break
				}
			}
			if target != "" {
				break
			}
		}
	}
	if target == "" {
		return ""
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(p.Cwd, target)
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
