package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Harness hooks are the enforcement layer: they answer "may this session modify files?"
// before the tool runs, and most can refuse.
//
// The shapes below are taken from each project's own documentation and config schema,
// not from memory — the survey behind them read the installed packages, the published
// schemas, and the vendored plugin type definitions.
//
// opencode and Zed are absent on purpose. opencode's tool.execute.before hook is typed to
// return Promise<void> and can only mutate arguments; Zed has no agent hook surface at
// all. Their gate is a permission rule, and `seraph hook` reports them as advisory rather
// than pretending otherwise.
var hookTargets = []struct {
	Name string
	Path string // relative to the project, or absolute when marked below
	// Project reports whether the file belongs in the repository (and so travels) or in
	// the user's home configuration (and so does not).
	Project bool
	// Doc is where the shape came from, shown by `seraph hook --explain`.
	Doc  string
	Home string
	// Version is the command that prints this harness's own version. `seraph hook
	// --verify` uses it to catch a verified entry that an upgrade has quietly
	// invalidated. Empty for a target with nothing to compare.
	Version []string
}{
	{
		Name: "Claude Code", Project: true,
		Path: ".claude/settings.json",
		Doc:  "code.claude.com/docs/en/hooks — hooks.PreToolUse, sync unless \"async\": true",
	},
	{
		Name: "Codex", Home: ".codex",
		Path: "hooks.json",
		Doc:  "learn.chatgpt.com/docs/hooks — same payload; fails OPEN on any hook error",
	},
	{
		Name: "Cursor", Project: true,
		Path: ".cursor/hooks.json",
		Doc:  "cursor.com/docs/hooks — hooks.preToolUse, {\"permission\":\"deny\"}; exit 2 also denies",
	},
	{
		Name: "Kiro", Project: true,
		Path: ".kiro/hooks/seraph-gate.json",
		Doc:  "kiro.dev/docs/hooks/types — exit 2 blocks and returns stderr to the model",
	},
	{
		Name: "Copilot CLI", Home: ".copilot",
		Path: "hooks/seraph-gate.json",
		Doc:  "docs.github.com copilot hooks-reference — preToolUse; fails OPEN on timeout only",
	},
	{
		// omp discovers hooks in ~/.omp/agent/hooks/, and in .claude/hooks/{pre,post} and
		// ~/.codex/hooks imported from other tools. It has no project hook directory at
		// all — the earlier ".omp/hooks/pre/" path was ours, not omp's, so the file that
		// install wrote there was never loaded by anything. See the package's own
		// examples/hooks/README.md, which says where to put one.
		Name: "omp", Home: ".omp",
		Path:    "agent/hooks/seraph-gate.ts",
		Doc:     "examples/hooks/permission-gate.ts in @oh-my-pi/pi-coding-agent — default-exported HookAPI, pi.on(\"tool_call\") returning {block:true}; discovered only in ~/.omp/agent/hooks/",
		Version: []string{"omp", "--version"},
	},
	{
		Name: "opencode", Home: ".config/opencode",
		Path: "opencode.json",
		Doc:  "NO HOOK CAN BLOCK — tool.execute.before returns Promise<void>. Advisory here; the gate for opencode is permission.edit",
		// advisory marks a harness we report but do not write.
	},
	{
		Name: "Zed", Home: ".config/zed",
		Path: "settings.json",
		Doc:  "NO HOOK SURFACE — agent.tool_permissions.tools.edit_file.always_deny is the gate",
	},
	{
		Name: "Gemini CLI", Home: ".gemini",
		Path: "settings.json",
		Doc:  "geminicli.com/docs/hooks — hooks.BeforeTool; synchronous; fails OPEN on non-JSON stdout",
	},
	{
		Name: "hermes", Home: ".hermes",
		Path: "config.yaml",
		Doc:  "{\"action\":\"block\",\"message\":\"…\"} on stdout; needs a shell-hooks allowlist entry",
	},
	{
		Name: "openclaw", Home: ".openclaw",
		Path: "extensions/seraph-gate/openclaw.plugin.json",
		Doc:  "registerHook(\"before_tool_call\") returning {block:true}; fail-closed",
	},
	{
		Name: "Cline", Home: ".cline",
		Path: "plugins/seraph-gate/plugin.json",
		Doc:  "AgentPlugin.hooks.beforeTool returning {skip:true}; failureMode fail_closed",
	},
}

// Unsupported lists the harnesses whose hooks cannot block, so `seraph hook` can say so
// plainly instead of reporting coverage it does not have.
var Unsupported = map[string]string{
	"opencode": "its pre-tool hook can only mutate arguments; use permission.edit: \"deny\"",
	"Zed":      "no agent hook surface; use agent.tool_permissions.tools.edit_file.always_deny",
	// Kiro earns its place here on evidence, not on the shape of the file. Seraph wrote
	// .kiro/hooks/seraph-gate.json and `seraph hook` reported it installed, while a write
	// from a kiro session holding no claim succeeded. kiro-cli 2.11.1 has no hook
	// mechanism: the only "PreToolUse" string in the binary is its own changelog entry
	// for the feature request "Add support for preToolUse and postToolUse hook" (#2875).
	// Unlike the two above there is no setting to point at instead — V3 announces hooks,
	// but its shape has not been read from the docs here, and inventing one is the exact
	// habit this task exists to stop. Revisit when Kiro CLI 3 is actually installed.
	"Kiro": "kiro-cli 2.x has no hook surface; V3 announces hooks whose shape is not established here",
}

// verification records a refusal actually observed from that harness: the build it was
// observed on, the date, and what was seen. Nothing else promotes a target to
// "enforcing" — no file, no doc, no changelog.
type verification struct {
	build string
	// version is the token `seraph hook --verify` looks for in the harness's own version
	// output, so an upgrade that invalidates the entry is caught rather than assumed away.
	version string
	date    string
	note    string
}

// verified records refusals somebody watched, not refusals a document predicted.
//
// Seraph has watched exactly one, and the asymmetry is the point. Feeding payloads to
// `seraph gate` proves the gate refuses them, which is a claim about the gate; it says
// nothing about whether a harness ever calls the gate. TASK-110 showed how far apart those
// two claims can be — a kiro session holding no claim wrote a file while `seraph hook`
// reported the gate installed there — and TASK-114 found omp's gate inert for longer still,
// written to a directory omp does not read.
var verified = map[string]verification{
	// The one entry, and it is here because somebody watched it happen rather than because
	// the documentation suggested it. On 2026-10-05 an omp 18.0.3 subprocess was told to run
	// `echo gateprobe > /tmp/omp-gate-probe2.txt`; the hook refused the shell call with the
	// gate's own message and the file was confirmed absent afterwards. That is a refusal from
	// the harness, not from a payload handed to `seraph gate` directly — the distinction every
	// other harness on this list is still waiting for.
	"omp": {build: "omp 18.0.3", version: "18.0.3", date: "2026-10-05", note: "gate written to a discovered hooks directory; observed refusing a claimless shell call"},
}

// VerifyCheck is one registry entry measured against the harness installed now.
type VerifyCheck struct {
	Harness   string
	Recorded  string
	Installed string
	Holds     bool
	Err       string
}

// VerifyRecorded reports whether each verified harness still runs the build its entry was
// witnessed on. It re-checks an entry; it does not re-earn one. A matching build still has
// only the one refusal behind it, and watching a new one is somebody's job with the harness
// in front of them.
func VerifyRecorded() []VerifyCheck {
	var checks []VerifyCheck
	for _, target := range hookTargets {
		v, ok := verified[target.Name]
		if !ok || len(target.Version) == 0 {
			continue
		}
		check := VerifyCheck{Harness: target.Name, Recorded: v.build}
		out, err := exec.Command(target.Version[0], target.Version[1:]...).Output()
		if err != nil {
			check.Err = err.Error()
		} else {
			check.Installed = strings.TrimSpace(string(out))
			check.Holds = versionHolds(v.version, check.Installed)
		}
		checks = append(checks, check)
	}
	return checks
}

// versionHolds reports whether a harness's own version output still contains the build the
// entry was witnessed on.
//
// Containment, not equality: binaries disagree about prefixes, and `omp --version` prints
// "omp v18.0.3" where the registry records "18.0.3". Comparing whole strings would report
// every harness as stale the moment its name appeared in the output, which is the false alarm
// that teaches people to ignore a warning.
func versionHolds(expected, installed string) bool {
	return expected != "" && strings.Contains(installed, expected)
}

// HookReport is one harness's state after install.
type HookReport struct {
	Name   string
	Path   string
	Status string
	Detail string
	Doc    string
}

// InstallHooks writes the gate into every harness that can enforce it, and reports the
// rest as advisory.
//
// mergeInto preserves a JSON config's existing keys; anything it cannot parse is left
// alone and reported, because these files hold credentials and a rewrite must never be
// the thing that destroys them.
func InstallHooks(root, seraphBin string, dryRun bool) ([]HookReport, error) {
	var reports []HookReport
	for _, target := range hookTargets {
		path := target.Path
		if !target.Project {
			home, err := os.UserHomeDir()
			if err != nil {
				reports = append(reports, HookReport{target.Name, path, "skipped", "no home directory", target.Doc})
				continue
			}
			path = filepath.Join(home, target.Home, target.Path)
		} else {
			path = filepath.Join(root, target.Path)
		}

		if unsupported, ok := Unsupported[target.Name]; ok {
			reports = append(reports, HookReport{target.Name, path, "advisory only", unsupported, target.Doc})
			continue
		}

		status, detail, err := writeHook(target.Name, path, seraphBin, dryRun)
		if err != nil {
			reports = append(reports, HookReport{target.Name, path, "refused", err.Error(), target.Doc})
			continue
		}
		reports = append(reports, HookReport{target.Name, path, status, detail, target.Doc})
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Name < reports[j].Name })
	return reports, nil
}

func writeHook(harness, path, seraphBin string, dryRun bool) (string, string, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "refused", "", fmt.Errorf("read: %w", err)
	}

	switch harness {
	case "omp":
		body := ompHookSource(seraphBin)
		if dryRun {
			return "would install", "TypeScript hook module", nil
		}
		if string(existing) == body {
			return "installed", "unchanged", nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "refused", "", err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return "refused", "", err
		}
		return "installed", "TypeScript hook module", nil

	case "hermes":
		// config.yaml is not JSON, and rewriting a 14 kB hand-maintained YAML is exactly
		// the failure this project refuses to cause. Say so and leave it alone.
		return "manual", "config.yaml is hand-edited — add the hook yourself (`seraph hook --explain`)", nil

	case "openclaw", "Cline":
		return "manual", "needs a plugin package — `seraph hook --explain` has the shape", nil

	default:
		return mergeJSONHook(harness, path, existing, seraphBin, dryRun)
	}
}

func mergeJSONHook(harness, path string, existing []byte, seraphBin string, dryRun bool) (string, string, error) {
	doc := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &doc); err != nil {
			return "refused", "", fmt.Errorf("exists but is not valid JSON (%v) — left untouched", err)
		}
	}

	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	switch harness {
	case "Gemini CLI":
		if hasGeminiGate(hooks) {
			return "installed", "unchanged", nil
		}
		hooks["BeforeTool"] = append(list(hooks["BeforeTool"]),
			map[string]any{
				"matcher": matcherFor(harness),
				"hooks": []any{map[string]any{
					"name":    "seraph-gate",
					"type":    "command",
					"command": gateCommand(seraphBin, "gemini"),
					"timeout": 10,
				}},
			})
	default:
		if hasDefaultGate(hooks) {
			return "installed", "unchanged", nil
		}
		hooks["PreToolUse"] = append(list(hooks["PreToolUse"]),
			map[string]any{
				"matcher": matcherFor(harness),
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": gateCommand(seraphBin, harness),
				}},
			})
	}
	doc["hooks"] = hooks

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "refused", "", err
	}
	if string(out) == string(existing) {
		return "installed", "unchanged", nil
	}
	if dryRun {
		return "would install", "PreToolUse gate added", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "refused", "", err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return "refused", "", err
	}
	return "installed", "PreToolUse gate added", nil
}

// matcherFor covers the write tools and the shell. Leaving the shell out would make the
// gate advisory with extra machinery: `bash -c 'echo x > file'` fires no Write hook.
func matcherFor(harness string) string {
	switch harness {
	case "Kiro":
		return "write|edit|patch|bash|terminal|execute|run_command"
	case "omp":
		return "*"
	default:
		return "Write|Edit|MultiEdit|NotebookEdit|apply_patch|Bash|EditNotebook|BashOutput"
	}
}

func gateCommand(seraphBin, harness string) string {
	name := strings.ToLower(strings.ReplaceAll(harness, " ", ""))
	return fmt.Sprintf("%s gate --harness %s", quote(seraphBin), name)
}

func quote(path string) string {
	if strings.ContainsAny(path, " \t") {
		return `"` + path + `"`
	}
	return path
}

func list(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

func hasDefaultGate(hooks map[string]any) bool {
	for _, entry := range list(hooks["PreToolUse"]) {
		if mentionsSeraph(entry) {
			return true
		}
	}
	return false
}

func hasGeminiGate(hooks map[string]any) bool {
	for _, entry := range list(hooks["BeforeTool"]) {
		if mentionsSeraph(entry) {
			return true
		}
	}
	return false
}

func mentionsSeraph(v any) bool {
	encoded, err := json.Marshal(v)
	return err == nil && strings.Contains(string(encoded), "seraph gate")
}

// installedBy reports whether a target's file is the gate this package wrote.
//
// Every JSON-config target stores the hook command as one string, so mentionsSeraph finds
// it. The omp hook is not a config: it is a TypeScript module that spawns the binary with
// ["gate", "--harness", "omp"] and keeps the binary's path in a separate const, so the two
// words never appear next to each other. Matching it with the config predicate made
// `seraph hook` report a gate that install had just written as not installed — the health
// report contradicting the installer, which is the one failure mode this project exists to
// end. The module is matched instead on the spawn call the generated module always carries and a
// hand-written one almost never does — the earlier two-string probe accepted any file containing
// the marker and the env var, which is a false "installed" for a module that never runs the gate.
func installedBy(harness, raw string) bool {
	if harness == "omp" {
		return strings.Contains(raw, "Installed by seraph.") &&
			strings.Contains(raw, `"gate", "--harness", "omp"`)
	}
	return mentionsSeraph(raw)
}

// ompHookSource renders the hook in the shape omp actually ships, taken from its own
// examples/hooks/permission-gate.ts rather than from memory.
//
// Two details are load-bearing and both were wrong before. The module imports HookAPI from
// the package root and default-exports a plain function taking it: ExtensionFactory belongs
// to the EXTENSIONS subsystem and does not resolve here. And it must land in
// ~/.omp/agent/hooks/, the only native directory omp reads — see the omp target above.
//
// That class of mistake fails in the worst available way. The module does not throw where
// an agent would notice; the harness logs the load error, carries on without the hook, and
// the agent writes files exactly as if no gate existed. The health report agreed the whole
// time, because it was checking that our file existed.
func ompHookSource(seraphBin string) string {
	return fmt.Sprintf(`// Installed by seraph. Refuses a write from a session holding no live claim.
import { spawnSync } from "node:child_process";
import type { HookAPI } from "@oh-my-pi/pi-coding-agent";

const GATE = %s;
const READ_ONLY = /^(read|grep|glob|search|list|ls|cat|webfetch|websearch|fetch|todo)/i;

export default function (pi: HookAPI) {
  pi.on("tool_call", async (event, ctx) => {
    const toolName = String(event.toolName ?? "");
    if (READ_ONLY.test(toolName)) return undefined;

    const proc = spawnSync(GATE, ["gate", "--harness", "omp"], {
      input: JSON.stringify({
        tool_name: toolName,
        tool_input: event.input ?? {},
        cwd: ctx?.cwd ?? process.cwd(),
        session_id: process.env.SERAPH_SESSION_ID ?? "",
      }),
      encoding: "utf8",
    });

    if (proc.status === 2) {
      const reason = String(proc.stderr ?? "").trim();
      return { block: true, reason: reason || "seraph: no live claim on this board" };
    }
    return undefined;
  });
}
`, tsString(seraphBin))
}

// tsString renders a Go path as a TypeScript string literal.
//
// A bare path is not safe here: an unquoted /home/... is parsed as a regular
// expression literal, and the failure surfaces as a dozen unrelated "invalid flag"
// build errors that name neither the path nor the real cause.
func tsString(path string) string {
	return strconv.Quote(filepath.ToSlash(path))
}

// GateCommand exposes the exact command a hook runs, for documentation and debugging.
func GateCommand(seraphBin, harness string) string {
	return gateCommand(seraphBin, harness)
}

// SelfPath resolves the running seraph binary, so the hook it installs points at this
// build rather than whatever is on PATH.
func SelfPath() string {
	exe, err := exec.LookPath("seraph")
	if err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			return abs
		}
		return exe
	}
	return "seraph"
}

// InspectHooks reports each harness's current state without writing anything.
func InspectHooks(root string) ([]HookReport, error) {
	var reports []HookReport
	for _, target := range hookTargets {
		path := target.Path
		if !target.Project {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			path = filepath.Join(home, target.Home, target.Path)
		} else {
			path = filepath.Join(root, target.Path)
		}
		status, detail := "not installed", ""
		if reason := Unsupported[target.Name]; reason != "" {
			status, detail = "cannot enforce", reason
			// An earlier install may have written this harness a gate file it never
			// reads. Naming it costs nothing; deleting a file the user may rely on is
			// not this command's call.
			if raw, err := os.ReadFile(path); err == nil && installedBy(target.Name, string(raw)) {
				detail += "; a gate file written by an earlier install is still at " + path + " and is inert"
			}
		} else if raw, err := os.ReadFile(path); err == nil && installedBy(target.Name, string(raw)) {
			if v, ok := verified[target.Name]; ok {
				status, detail = "enforcing (verified "+v.build+", "+v.date+")", v.note
			} else {
				status = "installed, unverified"
			}
		}
		reports = append(reports, HookReport{target.Name, path, status, detail, target.Doc})
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Name < reports[j].Name })
	return reports, nil
}
