package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `seraph hook` is the health report a reader trusts, so its vocabulary is a contract.
// "installed" used to mean only that a file Seraph wrote was still on disk, which is how a
// harness that never reads that file came to read as covered.

// existingBinary is a path that is genuinely there, so a fixture describing a healthy hook
// is not read as a dead one. The health report now asks whether the command a hook carries
// would actually run, and a made-up path is indistinguishable from the defect it looks for.
func existingBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve this test binary: %v", err)
	}
	return exe
}

// gateFile renders a Claude-shaped settings.json whose gate runs binary.
func gateFile(binary string) string {
	return `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Edit|Bash",
        "hooks": [
          {"type": "command", "command": "` + binary + ` gate --harness claude"}
        ]
      }
    ]
  }
}`
}

// newRepo lays down a project root holding exactly the hook files a case needs, and points
// $HOME at a scratch directory so user-scope targets cannot read this machine's real config.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func statusOf(t *testing.T, root, harness string) HookReport {
	t.Helper()

	reports, err := InspectHooks(root)
	if err != nil {
		t.Fatalf("InspectHooks: %v", err)
	}
	for _, r := range reports {
		if r.Name == harness {
			return r
		}
	}
	t.Fatalf("no report for %s", harness)
	return HookReport{}
}

func TestAPresentGateFileDoesNotReadAsEnforcing(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile(existingBinary(t))})

	got := statusOf(t, root, "Claude Code")

	if got.Status != "installed, unverified" {
		t.Errorf("a file on disk must not read as enforcement, got %q", got.Status)
	}
}

func TestAMissingFileReadsAsNotInstalled(t *testing.T) {
	root := newRepo(t, nil)

	if got := statusOf(t, root, "Claude Code"); got.Status != "not installed" {
		t.Errorf("expected not installed, got %q", got.Status)
	}
}

// The state has to be reachable, or the honest answer is a dead end. Nothing grants it but
// an entry recording a refusal somebody actually watched.
func TestEnforcingRequiresARecordedRefusal(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile(existingBinary(t))})

	if got := statusOf(t, root, "Claude Code"); strings.Contains(got.Status, "enforcing") {
		t.Fatalf("precondition: %q already claims enforcement", got.Status)
	}

	verified["Claude Code"] = verification{build: "2.1.84", date: "2026-10-05", note: "watched it refuse"}
	t.Cleanup(func() { delete(verified, "Claude Code") })

	got := statusOf(t, root, "Claude Code")
	if !strings.Contains(got.Status, "enforcing (verified 2.1.84, 2026-10-05)") {
		t.Errorf("expected the recorded refusal in the status, got %q", got.Status)
	}
	if got.Detail != "watched it refuse" {
		t.Errorf("expected the recorded note in the detail, got %q", got.Detail)
	}
}

// Kiro is the harness that proved the gap: the file was present, the report said installed,
// and an unclaimed write went through. Its leftover file must be named, not deleted.
func TestAnUnsupportedHarnessNamesItsLeftoverInertFile(t *testing.T) {
	leftover := `{"hooks":{"PreToolUse":[{"matcher":"write","hooks":[{"type":"command","command":"/usr/bin/seraph gate --harness kiro"}]}]}}`
	root := newRepo(t, map[string]string{".kiro/hooks/seraph-gate.json": leftover})

	got := statusOf(t, root, "Kiro")

	if got.Status != "cannot enforce" {
		t.Fatalf("expected cannot enforce, got %q", got.Status)
	}
	if !strings.Contains(got.Detail, "inert") {
		t.Errorf("expected the leftover to be named as inert, got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, got.Path) {
		t.Errorf("expected the leftover path %q in the detail, got %q", got.Path, got.Detail)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Errorf("the leftover must be reported, not deleted: %v", err)
	}
}

func TestAnUnsupportedHarnessWithNoLeftoverSaysOnlyWhy(t *testing.T) {
	root := newRepo(t, nil)

	got := statusOf(t, root, "Kiro")

	if got.Status != "cannot enforce" {
		t.Fatalf("expected cannot enforce, got %q", got.Status)
	}
	if strings.Contains(got.Detail, "inert") {
		t.Errorf("nothing is on disk, so nothing may be called inert, got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "no hook surface") {
		t.Errorf("expected the reason to survive, got %q", got.Detail)
	}
}

// The whole point of --verify is to catch an entry that an upgrade has invalidated, so the
// comparison has to survive the ways real binaries format their version while still failing
// on a genuinely different build.
func TestVersionHoldsIsContainmentNotEquality(t *testing.T) {
	// `omp --version` prints "omp/18.0.3"; the registry records "18.0.3".
	if !versionHolds("18.0.3", "omp/18.0.3") {
		t.Error("an entry must hold when the binary reports the same version in its own format")
	}
	if !versionHolds("2.1.84", "2.1.84 (Claude Code)") {
		t.Error("a trailing description must not read as a mismatch")
	}
	if versionHolds("18.0.3", "omp/19.0.0") {
		t.Error("an upgraded harness must not pass as the build the entry was witnessed on")
	}
	if versionHolds("18.0.3", "") {
		t.Error("a harness that reported nothing must not read as holding")
	}
	if versionHolds("", "omp/18.0.3") {
		t.Error("an entry with no version token can never be shown to hold")
	}
}

// The omp hook is the only place a session's real identity can be found: omp starts the
// seraph server with no SERAPH_SESSION_ID in its environment, so the agent cannot know the
// id the gate reads, and a claim made under any other id is refused as though nothing were
// held. The generated module must read the id from the harness's own session manager and
// put it in both the gate payload and seraph's lifecycle calls.
func TestTheOmpHookCarriesTheHarnessSessionId(t *testing.T) {
	src := ompHookSource("/home/someone/go/bin/seraph")

	for _, want := range []string{
		"ctx?.sessionManager?.getSessionId?.()",
		"session_id: sessionId",
		`["claim_task", "update_task", "release_task"]`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated omp hook is missing %q; a claim and the gate would name different sessions", want)
		}
	}

	// The revision must reach the payload the tool actually runs with, not just the gate.
	if !strings.Contains(src, "return { input: stamped }") {
		t.Error("generated omp hook never revises a lifecycle call's input with the session id")
	}

	// TASK-106's detection depends on this exact shape. Losing it would report an installed
	// gate as missing the moment the template changed.
	if !installedBy("omp", src) {
		t.Error("the generated omp hook no longer satisfies installedBy, so `seraph hook` would regress")
	}
}

// The three hooks install wrote on this machine pointed at /tmp/binp/seraph, which is gone.
// `seraph hook` called them installed and re-running install could not repair them, so a
// command that cannot run read exactly like coverage.
func TestAHookPointingAtAMissingBinaryReadsAsUnrunnable(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile("/nonexistent/binp/seraph")})

	got := statusOf(t, root, "Claude Code")

	if !strings.Contains(got.Status, "command missing") {
		t.Fatalf("a hook whose binary is gone must not read as installed, got %q", got.Status)
	}
	if !strings.Contains(got.Detail, "/nonexistent/binp/seraph") {
		t.Errorf("the report must name the path that does not run, got %q", got.Detail)
	}
}

// A hook naming a bare command is resolved on PATH; one naming an absent command is dead.
func TestABareCommandIsResolvedOnPath(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile("definitely-not-a-real-binary-xyz")})

	if got := statusOf(t, root, "Claude Code"); !strings.Contains(got.Status, "command missing") {
		t.Errorf("a bare command that is not on PATH must read as unrunnable, got %q", got.Status)
	}
}

// A moved binary is the same defect one step earlier: the hook is intact, the command it
// carries is not. install must rewrite it rather than report unchanged.
func TestInstallRepairsACommandPointingAtAnotherBinary(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile("/nonexistent/binp/seraph")})

	want := existingBinary(t)
	if _, err := InstallHooks(root, want, false); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, ".claude/settings.json"))
	if err != nil {
		t.Fatalf("read the rewritten config: %v", err)
	}
	if !strings.Contains(string(raw), want) {
		t.Errorf("install left the dead command in place:\n%s", raw)
	}
	if strings.Contains(string(raw), "/nonexistent/binp/seraph") {
		t.Errorf("the dead path survived an install:\n%s", raw)
	}
	if got := statusOf(t, root, "Claude Code"); !strings.Contains(got.Status, "installed") {
		t.Errorf("a repaired hook must read as installed, got %q", got.Status)
	}
}

// Repair must be idempotent: a second install over a healthy entry changes nothing.
func TestRepairIsIdempotent(t *testing.T) {
	root := newRepo(t, map[string]string{".claude/settings.json": gateFile("/nonexistent/binp/seraph")})

	want := existingBinary(t)
	if _, err := InstallHooks(root, want, false); err != nil {
		t.Fatalf("first InstallHooks: %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(root, ".claude/settings.json"))

	reports, err := InstallHooks(root, want, false)
	if err != nil {
		t.Fatalf("second InstallHooks: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(root, ".claude/settings.json"))

	if string(first) != string(second) {
		t.Errorf("a second install rewrote a healthy file:\n%s\n---\n%s", first, second)
	}
	for _, r := range reports {
		if r.Name == "Claude Code" && r.Detail != "unchanged" {
			t.Errorf("expected unchanged on the second pass, got %q", r.Detail)
		}
	}
}

// Gemini measures its hook timeout in milliseconds and names tools write_file/replace/
// run_shell_command. The installed entry said 10, which is ten milliseconds — a hook killed
// before it could answer — and carried Claude tool names, which Gemini never emits, so the
// matcher could not fire.
func TestGeminiHookUsesMillisecondsAndGeminiToolNames(t *testing.T) {
	root := newRepo(t, nil)

	if _, err := InstallHooks(root, existingBinary(t), false); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	home, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("read the Gemini config: %v", err)
	}
	body := string(raw)

	if !strings.Contains(body, `"timeout": 10000`) {
		t.Errorf("Gemini's timeout is milliseconds and must not be 10:\n%s", body)
	}
	if !strings.Contains(body, "run_shell_command") {
		t.Errorf("the matcher must name a tool Gemini emits:\n%s", body)
	}
	if strings.Contains(body, "MultiEdit") || strings.Contains(body, "EditNotebook") {
		t.Errorf("the Gemini matcher still carries Claude tool names:\n%s", body)
	}
	if !strings.Contains(body, "BeforeTool") {
		t.Errorf("Gemini reads hooks under BeforeTool:\n%s", body)
	}
}

// Cursor reads its own schema from .cursor/hooks.json: a camelCase preToolUse, a flat entry,
// and a version. The PascalCase PreToolUse install used to write is Claude's shape, which
// Cursor honours only from .claude/settings*.json, so the file said gated and gated nothing.
func TestCursorHookIsWrittenInCursorsOwnSchema(t *testing.T) {
	root := newRepo(t, nil)

	if _, err := InstallHooks(root, existingBinary(t), false); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("read the Cursor config: %v", err)
	}
	body := string(raw)

	if !strings.Contains(body, `"version": 1`) {
		t.Errorf("Cursor's schema requires a version:\n%s", body)
	}
	if !strings.Contains(body, `"preToolUse"`) {
		t.Errorf("Cursor reads the camelCase event name:\n%s", body)
	}
	if strings.Contains(body, `"PreToolUse"`) {
		t.Errorf("the Claude-shaped key must not be written into Cursor's own file:\n%s", body)
	}
	if !strings.Contains(body, `"matcher": "Shell|Write"`) {
		t.Errorf("the matcher must name tools Cursor emits:\n%s", body)
	}
	if got := statusOf(t, root, "Cursor"); !strings.Contains(got.Status, "installed") {
		t.Errorf("the Cursor hook must read as installed, got %q", got.Status)
	}
}

// An earlier install left the gate under Claude's event name inside Cursor's file. That
// entry is inert there, and Cursor does not read it — leaving it would keep a dead hook
// claiming to be the gate.
func TestCursorInstallRemovesTheClaudeShapedLeftover(t *testing.T) {
	leftover := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "/nonexistent/binp/seraph gate --harness cursor"}]}
    ]
  }
}`
	root := newRepo(t, map[string]string{".cursor/hooks.json": leftover})

	if _, err := InstallHooks(root, existingBinary(t), false); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("read the rewritten Cursor config: %v", err)
	}
	body := string(raw)

	if strings.Contains(body, `"PreToolUse"`) {
		t.Errorf("the inert Claude-shaped entry must be removed, not left beside the live one:\n%s", body)
	}
	if !strings.Contains(body, `"preToolUse"`) {
		t.Errorf("the live entry must remain:\n%s", body)
	}
}
