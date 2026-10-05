// Package gate decides whether a harness may modify files: it answers the question a
// pre-tool hook asks — "this session is about to write; does it hold a live claim?"
//
// It is a separate binary because hooks execute a command, and that command must be fast,
// must not open a database a parent process already has open, and must not depend on the
// harness having loaded Seraph as an MCP server. It therefore answers from the same SQLite
// file the board lives in, read-only.
package gate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Decision is the machine-readable answer a hook consumes.
type Decision struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Task   string `json:"task,omitempty"`
}

// writeTools and execTools are matched as substrings against the harness's tool name, which
// differs per harness ("Write", "apply_patch", "edit_file", "Bash", "exec", "terminal").
//
// The exec tools matter as much as the write tools. A gate that matched only Write could
// be walked around with `bash -c 'echo x > file'`, which would leave the board lying about
// what was done — the exact failure this is meant to end. Gating exec is stricter, and
// that is the trade being made: shell use now requires a live claim too.
var writeTools = []string{"write", "edit", "patch", "create", "delete", "move", "rename", "apply"}
var execTools = []string{"bash", "exec", "shell", "terminal", "command", "run_command", "powershell"}

// readTools never need a claim. A harness is entitled to look before it leaps.
var readTools = []string{"read", "grep", "glob", "search", "list", "ls", "cat", "webfetch", "websearch", "fetch"}

// exemptPaths are paths a hook must not block, or Seraph would prevent its own snapshots
// from being written and the harness would deadlock against a board it cannot refresh.
var exempt = []string{
	"/.seraph/",
	"/node_modules/",
	"/.git/",
}

// Verdict is the whole answer for one tool call.
type Verdict struct {
	Gate   Decision
	Reason string // why it decided that, for --explain
}

// seraphTools are the board's own lifecycle tools. They are never gated.
//
// This was a real deadlock: "create" appears in writeTools, so mcp__seraph_create_task was
// classified as a write and refused for want of a claim — but claiming requires creating
// first. The agent could not create the task that would unblock it, and every write stayed
// locked. A gate must never obstruct the mechanism that satisfies it.
var seraphTools = []string{"mcp__seraph_", "seraph_", "xd://mcp__seraph"}

// Evaluate decides whether a tool call named tool may run inside dir for session.
func Evaluate(root, tool, session string, toolInput any) Verdict {
	// A harness may dispatch MCP tools through its own write tool, passing an xd://
	// device path as the file argument. The tool name is then the harness's own, and the
	// real target is in the payload. Checking only the tool name refused
	// mcp__seraph_create_task on its way to the board — the one call that would have
	// unblocked the agent.
	if isSeraphDevice(tool, toolInput) {
		return Verdict{Gate: Decision{Allow: true}, Reason: "seraph's own tool"}
	}
	lower := strings.ToLower(tool)

	if containsAny(lower, readTools) {
		return Verdict{Gate: Decision{Allow: true}, Reason: "read-only tool"}
	}
	isWrite := containsAny(lower, writeTools)
	isExec := containsAny(lower, execTools)
	if !isWrite && !isExec {
		return Verdict{Gate: Decision{Allow: true}, Reason: "not a write or exec tool"}
	}
	kind := "write"
	if isExec {
		kind = "exec"
	}

	// The hook is not told which session is asking: harnesses do not put one in the
	// payload, and the session id is the agent's own choice when it calls claim_task. So
	// with no session the gate enforces the rule it can actually check — that the board
	// holds a live claim — rather than a per-session rule it would have to guess at, which
	// would refuse every write forever.
	c, err := liveClaims(root, session)
	if err != nil {
		return Verdict{Gate: Decision{Allow: true}, Reason: "board unreadable: " + err.Error() + " — allowing, so a database problem cannot freeze your work"}
	}

	if c.held != "" {
		return Verdict{Gate: Decision{Allow: true, Task: c.held}, Reason: "session already holds " + c.held}
	}
	if session == "" && len(c.unclaimed) == 0 && someClaimExists(root) {
		// Someone holds a claim but the hook cannot attribute it. Refusing here would be
		// a guess, and a wrong guess is a deadlock.
		return Verdict{Gate: Decision{Allow: true}, Reason: "another session holds a claim; this hook cannot tell which"}
	}

	// A claim this session owns under another identity was indistinguishable from an
	// empty board: both were told the session held nothing, and both read as a policy
	// decision rather than an identity mismatch. Say how much is held elsewhere, so the
	// caller can tell the two cases apart.
	elsewhere := ""
	if c.elsewhere > 0 {
		elsewhere = fmt.Sprintf(
			" %d task(s) on this board are held by another session; if that is you, your claim was taken "+
				"under a different session id than the one this gate reads.",
			c.elsewhere)
	}

	if isExec {
		// Refusing every shell command would block builds and tests behind a task,
		// which is the friction that pushes agents around the board entirely.
		return Verdict{
			Gate: Decision{Allow: false, Reason: fmt.Sprintf(
				"seraph: this is a %s tool and this session holds no task. %d task(s) are unclaimed on this board — "+
					"claim one with claim_task, or add one with create_task, then retry. Reading and searching are always allowed."+
					elsewhere,
				kind, len(c.unclaimed))},
			Reason: "exec with no live claim",
		}
	}

	return Verdict{
		Gate: Decision{Allow: false, Reason: fmt.Sprintf(
			"seraph: this is a %s tool and this session holds no task. Record the work first: create_task with a title, "+
				"a goal, and acceptance criteria, then claim_task. %d task(s) are unclaimed on this board."+elsewhere,
			kind, len(c.unclaimed))},
		Reason: kind + " with no live claim",
	}
}

// claims is what one caller needs to know about the board: what it may claim, what it
// already holds, and how much somebody else is holding.
//
// elsewhere is the third bucket, and it exists because of a live observation. A task held
// by a different live session matched neither "unclaimed" nor "held", so it fell out of
// both lists and the caller heard only that it held nothing — the same sentence a session
// gets on an empty board.
type claims struct {
	unclaimed []string
	held      string
	elsewhere int
}

// liveClaims reads the board once and sorts every live task into one of those three.
func liveClaims(root, session string) (claims, error) {
	var c claims
	path := filepath.Join(root, ".seraph", "state.db")
	if _, err := os.Stat(path); err != nil {
		return c, nil
	}

	dsn := "file:" + path +
		"?_pragma=query_only(1)&_pragma=busy_timeout(2000)&_txlock=deferred"
	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return c, err
	}
	defer handle.Close()

	// COALESCE on every nullable column. Scanning a raw NULL into an int64 errors, and
	// the error path here allows rather than blocks — so a missing COALESCE does not
	// announce itself, it quietly turns the gate off.
	rows, err := handle.Query(`SELECT id,
	                                 COALESCE(claim_session, ''),
	                                 COALESCE(claim_expires, 0)
	                           FROM tasks
	                           WHERE status != 'done'
	                           ORDER BY id`)
	if err != nil {
		return c, err
	}
	defer rows.Close()

	now := time.Now().UTC().Unix()
	for rows.Next() {
		var id, sess string
		var expires int64
		if err := rows.Scan(&id, &sess, &expires); err != nil {
			return c, err
		}
		switch {
		case sess == "" || expires <= now:
			c.unclaimed = append(c.unclaimed, id)
		case sess == session:
			c.held = id
		default:
			c.elsewhere++
		}
	}
	if err := rows.Err(); err != nil {
		return c, err
	}
	return c, nil
}

// isSeraphDevice reports whether a call is one of the board's own tools, whether it
// arrives by name or through a harness's write tool aimed at an xd:// device path.
func isSeraphDevice(tool string, input any) bool {
	if containsAny(strings.ToLower(tool), seraphTools) {
		return true
	}
	if input == nil {
		return false
	}
	m, ok := input.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"path", "device", "target", "uri"} {
		if v, ok := m[key].(string); ok && containsAny(strings.ToLower(v), seraphTools) {
			return true
		}
	}
	return false
}

func containsAny(lower string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}

// PathExempt reports whether a path is one a hook must not block.
func PathExempt(path string) bool {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		// Harnesses pass relative paths constantly, and every entry in exempt carries a
		// leading slash so it can only match a whole path segment. Without this, a
		// relative ".seraph/KANBAN.md" matched nothing and the gate blocked the very
		// snapshot write the list exists to protect — the deadlock, reached by a
		// shorter path. Found by TestSeraphSnapshotsAreExempt on its first run.
		slashed = "/" + slashed
	}
	for _, e := range exempt {
		if strings.Contains(slashed, e) {
			return true
		}
	}
	return false
}

// someClaimExists reports whether any live claim exists on the board, used only when the
// hook cannot identify the caller.
func someClaimExists(root string) bool {
	path := filepath.Join(root, ".seraph", "state.db")
	if _, err := os.Stat(path); err != nil {
		return false
	}
	handle, err := sql.Open("sqlite", "file:"+path+"?_pragma=query_only(1)&_pragma=busy_timeout(2000)")
	if err != nil {
		return false
	}
	defer handle.Close()
	now := time.Now().UTC().Unix()
	var n int
	if err := handle.QueryRow(
		`SELECT count(*) FROM tasks WHERE claim_session IS NOT NULL AND claim_session != '' AND claim_expires > ?`,
		now).Scan(&n); err != nil {
		return false
	}
	return n > 0
}
