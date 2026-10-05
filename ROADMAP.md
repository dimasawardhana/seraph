# Seraph v1 — Build Roadmap

Derived from [prd.md](./prd.md) v2.0.0. Every phase ends in a check you can run. If a
phase's check fails, fix it before starting the next one — the phases are ordered so that
each one is cheap to verify before the next one depends on it.

Estimates are rough and for pacing only.

---

## Definition of done — v1

- [x] Six tools: `get_board_summary`, `get_task`, `create_task`, `claim_task`, `update_task`, `release_task`
- [x] A tool response is byte-identical to the corresponding rendered snapshot
- [x] `done` is refused without a live claim held by the calling session
- [x] Claims expire, and an expired claim can be taken over
- [x] `seraph install` writes `.mcp.json`, both rule files, and `.seraph/.gitignore`, prints a diff, and is idempotent
- [x] `seraph doctor` reports repo root, database health, snapshot freshness, and per-harness registration
- [x] Two different harnesses coordinate on one task in one repository — [earned 2026-10-05](#run-of-2026-10-05--two-different-harness-products): kiro-cli 2.11.1 and omp, two separate products, coordinated on TASK-104

Nothing else. v1 is the six tools above plus `install` and `doctor`: the surface a harness
uses. Document search is **not** in v1 ([ADR-0004](./docs/adr/0004-seraph-indexes-prose-not-code.md),
§2 of the spec). The tree also ships the claim gate and the human-facing surfaces (`seraph board`,
`seraph ui`) that this list deliberately does not cover — each is recorded under
[Carried forward](#carried-forward-not-scheduled) with the reason it is out of scope.

---

## Phase 0 — Foundation

**Build:** Go module and `main.go`; CLI dispatch (no args = serve, plus `install` and `doctor`); repo-root resolution walking up for `.git` or `.seraph/`; SQLite open with WAL, `foreign_keys`, `busy_timeout`; the `tasks` and `id_sequence` DDL from spec §6 with migrations; a minimal `doctor` that resolves the root, opens the database, applies migrations, and prints the paths. Packages: `internal/repo`, `internal/db`, `internal/mcp`, `internal/doctor`. `main.go` also dispatches `board`, `ui`, `hook`, `gate` and `version` today; each belongs to a carried-forward surface below.

**Check:** `go build` produces a binary. `seraph doctor` in an empty directory creates `.seraph/state.db` and exits 0. Piped through a real stdio session, `seraph` answers `initialize` and returns an empty `tools/list` without writing anything to stdout outside the protocol.

**Not in this phase:** any tool, any rendering, any file writing beyond the database.

**Why first:** you will hit a failure in the first hour of the real work, and this is what stops it from being an ambiguous blank board.

---

## Phase 1 — The read path

**Build:** the canonical render producing both `KANBAN.md` and `board.json` from one pass; the atomic write (temp file → flush → fsync → chmod 0644 → rename); `get_board_summary`, returning the rendered bytes. Packages: `internal/board`, `internal/mcp`.

**Check:** insert a row directly with `sqlite3`, then assert the tool's response and the bytes on disk are **identical**. Not similar — identical. Delete both, re-run, assert they match again.

**Not in this phase:** any mutation. No `board.json` format bikeshedding beyond what the render already needs.

**Why here and not later:** ADR-0001 says the rendered view *is* the product. Building mutations first and retrofitting the render is how you end up with tools that return rows and files that disagree — the exact defect that killed v1.0's premise. Every mutation in Phase 2 is then born returning the rendered view.

---

## Phase 2 — Lifecycle and enforcement

**Build:** `create_task` with in-transaction ID allocation; `claim_task`; `update_task`; `release_task`; claim expiry and takeover; the error taxonomy from spec §8 with holder payloads. Packages: `internal/board`, `internal/mcp`.

**Check**, all against a real database:

- A second `claim_task` from a different session on a live claim → `CLAIM_HELD_BY_ANOTHER_SESSION`, with the holder in the payload
- `update_task` to `done` without a claim → `CLAIM_REQUIRED_FOR_DONE`
- `update_task` to `done` with a matching claim → succeeds, and releases the claim
- An expired claim does not block a new `claim_task`
- A stale `claim_token` → `CLAIM_TOKEN_STALE`
- Every mutation returns bytes identical to the re-rendered snapshot

**Not in this phase:** triage, priority, `metadata`, `get_task`, `status_filter`.

**Watch for:** the async-render trap. If you are tempted to hand rendering off to a goroutine, read ADR-0001 again — two concurrent mutations can rename out of order and silently regress the board. This is the one bug in this project that fails without failing.

---

## Phase 3 — Install and rules

**Build:** `.mcp.json` merge with a PATH-resolved bare `command`; the marker-delimited Rules block into **both** `AGENTS.md` and `CLAUDE.md`; `.seraph/.gitignore`; the diff report; refusal to touch a config whose structure is unrecognised. Packages: `internal/installer`.

**Check:** in a scratch repo containing pre-existing user content in both rule files —

- `install` writes all four targets and prints a diff
- Content outside the markers is byte-identical afterwards
- A second `install` is a no-op (idempotent)
- A missing `AGENTS.md` is created; an existing one is not clobbered
- `.seraph/.gitignore` ignores `state.db*` and does not ignore the two snapshots

**Not in this phase:** user-scope registration for opencode, Zed, or hermes. v1 *reports* them; `doctor` in Phase 5 tells you they're unconfigured.

**Note:** the diff must be printed *before* the write and the command must be able to decline. Silently editing a file that holds someone's GitHub token is the failure mode here, not a malformed JSON key.

---

## Phase 4 — Prove the thesis

**No code.** This is the phase that decides whether v1 is the right product.

Open the same repository in two different harnesses. One task. A claims it. B is refused and can see who holds it. A moves it through to `done`. B sees the result on its next read.

Then do a day of real work through it — not a test repo.

**Stop condition — read this before pushing on.** If two harnesses will not coordinate, or the claim lifecycle turns out to be friction that harnesses route around, **stop and reconsider.** That is not a bug to grind through; it is the thesis failing to hold, and the honest response is to change the design rather than to harden a product nobody wants. Everything downstream depends on this phase passing, and nothing in the spec can tell you the answer in advance.

**Questions to answer, because the plan cannot:** Do harnesses read the Rules, or ignore them? Does anyone read `KANBAN.md`, or only call tools? Is claiming before work a feature or a tax?

**Run of 2026-10-02 — the mechanics passed; the thesis is not answered.** Two independent sessions, same harness product (omp), one real task, about eleven minutes on this repository's own board: A claimed `TASK-101`, B was refused with the holder in the payload, A moved it to `done`, and B saw it on its next read. Every step of the paragraph above worked, on real data, end to end.

That is not a pass. Both sessions were the same harness, the run was minutes rather than a day, no session was contended, and two of the three questions were not exercised. Nobody here read `KANBAN.md` as their source of truth, and nobody claimed under time pressure, so "feature or a tax" has no evidence in either direction. Only the first question got an answer, and it holds for one harness, once.

The transcript — session identities, the verbatim `CLAIM_HELD_BY_ANOTHER_SESSION` payload, the final read — is on the board as `TASK-102`. Re-run this phase with a participant that is not omp (`doctor` already reports Claude Code and opencode as present but unregistered) and a full day of work before treating it as passed.

**Second attempt, same day — the gate is real now; the second harness is not available.** `seraph install` wired the claim gate into every harness here that was believed able to enforce one: Claude Code, Cursor and omp were added. Kiro was later removed from that list: on 2026-10-05 kiro-cli 2.11.1 was shown to enforce nothing, and `seraph hook` still reports its gate as installed (`TASK-110`). Codex, Copilot CLI and Gemini CLI were already in place. Verified by feeding the gate real payloads — a write from a session holding no claim exits 2 with `permissionDecision: deny`, and the same write from the session holding `TASK-104` exits 0 with `holding TASK-104`. opencode and Zed cannot enforce it at all, so for those two the gate stays advisory.

Registering a second harness product is blocked on something outside this repository, and both candidates have now been walked to their ends. `claude` answers `Not logged in · Please run /login`. opencode's configured default, `opencode/minimax-m2.1`, does not exist; changing it to `opencode/big-pickle` got further and then hit `OpenCode's free tier can only be used from within OpenCode` — in headless `opencode run` and in the TUI alike, so the model change does not unblock it. Every other route opencode offers is closed by account state rather than by configuration: `openrouter/*` → `Missing Authentication header`, `github-copilot/*` → `not licensed to use Copilot`, `opencode/*` on the paid tier → `Insufficient account funds`, `ollama` → not installed. opencode also carries no MCP servers at all, so Seraph is not wired into it either way. None of it is fixable without the person at the keyboard. Box 7 stays unchecked; `TASK-104` holds the attempt and `TASK-105` is the real work waiting for whoever logs a harness in.

Wiring the gate also surfaced a defect worth recording: `install` wrote the omp gate and reported it installed, while `hook` reported it not installed, forever. `mentionsSeraph` matched the literal `seraph gate`, which the generated TypeScript module never spells — it spawns `["gate", "--harness", "omp"]` with the binary path in a separate constant. The installer and the health report disagreed about a file on disk. Fixed as `TASK-106`.

---
### Run of 2026-10-05 — two different harness products

Box 7 is earned. The mechanics are the ones TASK-102 already recorded; what changed is that
the two sessions were different products. Harness A was kiro-cli 2.11.1, session
`ee6cfe57-ec4c-4638-a49b-9533d826fb42`. Harness B was omp, session
`omp-phase4-20261005T0336Z`. A registered seraph by importing `.mcp.json` at global scope,
read `AGENTS.md` and `.seraph/KANBAN.md` as files, claimed `TASK-104`, and set it
`in_progress`. B was given a prompt naming neither seraph nor the board; it read the same
instruction files, found the same task, attempted the claim, and was refused verbatim:

```json
{"reason":"CLAIM_HELD_BY_ANOTHER_SESSION","message":"TASK-104 is claimed by another session until 2026-10-05T04:05:26Z; do not start this work","holder":{"harness":"kiro","session":"ee6cfe57-ec4c-4638-a49b-9533d826fb42","expires":1791173126}}
```

B edited no files and did not pursue the hold by another route.

**The three questions, against this run:**

- *Do harnesses read the Rules, or ignore them?* **Yes — observed twice, across two products.**
  Neither session was prompted with the word seraph, the board, or any tool, and both found
  the workflow in `AGENTS.md`.
- *Does anyone read `KANBAN.md`, or only call tools?* **Yes — observed for the first time.**
  Kiro used `.seraph/KANBAN.md` as its source of truth and made no tool call to read the
  board. TASK-102 recorded the opposite: every participant drove the board through tools.
- *Is claiming before work a feature or a tax?* **Still not answered.** B stopped at the
  boundary and called it the correct one, but the run lasted minutes, was not under time
  pressure, and produced one agreeable observation. That is not evidence in either direction.

**What this run did not establish — the honest boundary of the pass:**

- **The day of real work did not happen.** This was a protocol run on this repository's own
  board, not a day of ordinary work. The second requirement of this phase is unmet.
- **The gate enforced nothing, in either participant.** `seraph hook` reports the Kiro gate
  installed; kiro-cli 2.11.1 enforces nothing, and a write from a session holding no claim
  succeeded (`TASK-110`). The refusal B met was a tool result it chose to respect, not a
  wall it hit. On this machine the gate is proven only by feeding `seraph gate` payloads
  directly.
- **A protocol weakness surfaced live.** `claim_task` accepts any `session_id` a caller
  supplies, while the gate reads `SERAPH_SESSION_ID`. A claim taken under the wrong
  identity produces a gate message indistinguishable from holding no claim at all.

**Verdict: box 7 earned; the stop condition not discharged.** That two harness products will
coordinate on one board rather than route around it held for one real contention between two
real products. Whether it holds across a day of ordinary work, and whether claiming is a
feature or a tax, are still open — and this phase should not be read as closed until someone
works this board for a day and says which.

---
## Phase 5 — Finish v1

**Build:** `get_task`; full `doctor` — harness detection, per-harness registration state, snapshot freshness, database health; the remaining edge cases the checks above surface. Packages: `internal/mcp`, `internal/doctor`, `internal/board`.

`status_filter` and `include_done` on `get_board_summary` were planned here and **were never built**. The shipped tool takes no arguments at all, so `get_board_summary` returns every status every time. They are recorded rather than quietly dropped, because they are the first thing a board of any size will want and the plan should not have to be re-derived. Corrected 2026-10-02, `TASK-107`.


**Check:** `seraph doctor` on this machine reports every harness it detects, distinguishes registered from not, and prints the block for the ones `install` deliberately does not touch. Every box in §Definition of done holds, with box 7 the recorded exception.

**Not in this phase:** document search, user-scope registration, the event log. All three are deferred with reasons recorded.

---

## Carried forward, not scheduled

### Shipped in the tree, not in v1

These exist under `internal/` and work, but no phase above builds them and the v1 list does not
count them. They are recorded here so the tree and this document say the same thing.

- **gate** — the pre-tool hook a harness runs (`seraph gate`), refusing a write from a session holding no live claim, plus `seraph hook` reporting which harnesses can wire one. v1 proves the claim *lifecycle*; enforcing it from inside each harness is integration work that belongs to whichever harnesses actually ship hooks, and Phase 4 is the evidence that would earn it a phase.
- **tui** — `seraph board`, the terminal board a human works. v1's consumer is the model, not the person; a human reads `KANBAN.md` and reaches the same lifecycle through the same tools.
- **webui** — `seraph ui`, the same board in a loopback browser behind a token. Same reason as `tui`, and it drives the existing tools rather than adding a second lifecycle.
- **dashboard** — the human session's board (claim, release, advance), shared by `tui` and `webui`. It exists to serve those two surfaces and is deliberately not reachable from MCP.
- **vocab** — the status and priority vocabulary and its labels, shared by `tui`, `webui`, and `dashboard` so all three speak the words the render does. Presentation, not contract.
- **version** — the version string for `seraph version` and the MCP handshake. A plain constant, not a phase.

### The Index — spec §13

Open questions, for whenever it becomes real:

- **Unit** — file, heading section, or fixed-size chunk
- **Trigger** — `install` only, or an explicit `reindex_docs` tool
- **Concurrency** — two harnesses reindexing the same repository at once

Already decided: corpus is prose only, and staleness is `content_hash` rather than mtime.

---

## How we work

One phase at a time. A phase is done when its check passes, not when its code is written. If a check reveals the plan was wrong, the plan changes and `prd.md` changes with it — the spec is a living document, not a contract we are honouring under duress.
