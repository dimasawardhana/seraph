---

# 📖 Project Specification: Seraph

**Version:** 2.0.0
**Supersedes:** 1.0.0 (restructured; see §14 for what changed and why)
**Status:** Ready to build
**Domain vocabulary:** [CONTEXT.md](./CONTEXT.md)
**Decisions of record:** [docs/adr/](./docs/adr/)

**Core architecture:** Single static Go binary · embedded SQLite (WAL) · MCP over stdio · synchronous atomic renderer

---

## 1. Executive Summary

**Seraph** is a durable, harness-independent board of a project's work. AI coding harnesses
stop keeping their own task state and read from and write to one local store, so that what a
project knows outlives the session that learned it.

Seraph is a Model Context Protocol server over stdio, a single static binary with no daemon,
no container, and no runtime service. It stores tasks in an embedded SQLite database and
renders them to plain-text files that any consumer — harness or human — can read.

**The thesis:** harnesses already have todos, and every one of them forgets. Three prior
attempts at a board in this environment all failed the same way, by storing status in prose
or in unconstrained text so that no one could trust it. Seraph's job is to make the state
machine real, and to make it shared.

---

## 2. Scope

### In v1

- Task lifecycle: create, claim, update, release.
- Claims with expiry, and completion gated on a held claim.
- Synchronous rendering to `.seraph/KANBAN.md` and `.seraph/board.json`.
- MCP server over stdio.
- `seraph install` — registers Seraph, injects Rules, prints a diff.
- `seraph doctor` — reports detected harnesses and registration state.

### Deliberately out of v1

| Excluded | Why |
| --- | --- |
| Document search (`docs_fts`, `search_context`) | A table and a reader with **no writer**. Its corpus is prose only — codegraph owns code permanently ([ADR-0004](./docs/adr/0004-seraph-indexes-prose-not-code.md)). Deferred whole, not stubbed — see §13. |
| Event log (`task_events`) | Was justified by "session playback," which is undesigned, and no tool reads it. It cost a write on every mutation to support nothing. |
| User-scope MCP registration | Six harnesses use six different config keys. v1 writes the committable project-scope file and *reports* the rest. `doctor` tells you what is missing. |

### v2

The **Index** — a searchable projection of project documents. This is what makes Seraph a
universal context layer rather than a unified board. See §13 for the design it needs.

---

## 3. Architecture Principles

1. **The rendered board is the source of truth.** Tools return the same bytes that get
   written to disk. A harness and a human reading the board at the same moment see the same
   thing. The database is the renderer's private store, not a surface consumers touch.
   → [ADR-0001](./docs/adr/0001-rendered-view-is-the-product.md)
2. **Writes are visible when they return.** A mutating call does not return until its
   rendering is in place. A caller may read the board immediately after changing it.
3. **Enforcement lives in the tool layer, not the Rules.** Instruction files are read by
   some harnesses and not others. A tool is in the harness regardless of what the harness
   was told to read. → [ADR-0003](./docs/adr/0003-completion-requires-a-claim.md)
4. **Static binary, no cgo, no build tags.** One build works on every platform, and every
   build has the same features. → [ADR-0002](./docs/adr/0002-pure-go-sqlite-driver.md)
5. **Never clobber.** Seraph merges into files it understands and reports the rest. It
   never overwrites a file it does not fully understand, and never writes a secret.

---

## 4. Directory Layout

```text
seraph/
├── go.mod
├── main.go
├── CONTEXT.md              # domain vocabulary — the source for naming
├── prd.md
├── docs/adr/               # decisions of record
└── internal/
    ├── db/
    │   ├── sqlite.go       # connection, WAL, pragmas
    │   └── schema.go       # DDL + migrations
    ├── board/
    │   ├── board.go        # task lifecycle, claims, invariants
    │   ├── render.go       # canonical rendering (markdown + json)
    │   └── write.go        # atomic snapshot write
    ├── repo/
    │   └── root.go         # repo-root resolution, .seraph/ bootstrap
    ├── installer/
    │   ├── install.go      # .mcp.json + Rules injection, diff report
    │   ├── detect.go       # harness detection (report-only in v1)
    │   └── rules.go        # the Rules block itself
    ├── doctor/
    │   └── doctor.go       # environment + registration report
    └── mcp/
        └── server.go       # stdio server + tool handlers
```

Per-repository state lives in:

<repo>/.seraph/
├── .gitignore              # written by `seraph install` — see below
├── state.db                # SQLite, WAL mode — machine-local
├── KANBAN.md               # rendered snapshot
└── board.json              # rendered snapshot

`.seraph/` replaces the v1 draft's `.context/`. That directory holds a *board*, and in this
vocabulary "context" means the durable record of a project's work and knowledge — a larger
thing than the directory holds. The rename is free today; nothing exists to migrate.

**`.seraph/` has two fates, and they differ by file.** `state.db` and its `-wal`/`-shm`
sidecars hold live claim state and are **machine-local — gitignored**. `KANBAN.md` and
`board.json` are **committed**, because they are the human-readable artifact and they are
the point of the project. `install` writes a self-contained `.seraph/.gitignore` rather than
editing the repository's root one, so a Seraph project never modifies a file the user may
care about:

```gitignore
# Seraph local state — machine-local, not for committing.
state.db
state.db-wal
state.db-shm
```

The directory-is-self-ignoring pattern comes from `codegraph`, which writes its own
`.codegraph/.gitignore`. Seraph **inverts** it: codegraph can ignore its whole directory
because it has no human-readable artifact, and Seraph cannot.

---

## 5. Domain Model

Full vocabulary in [CONTEXT.md](./CONTEXT.md). The terms that shape the schema:

| Term | Meaning | Values |
| --- | --- | --- |
| **Task** | A unit of tracked work | — |
| **Board** | Every task belonging to one repository | — |
| **Status** | Where a task sits on the line from unstarted to finished | `backlog` · `todo` · `in_progress` · `review` · `done` |
| **Triage** | Who can pick it up right now — a *different axis* from status | `needs-triage` · `needs-info` · `ready-for-agent` · `ready-for-human` · `wontfix` · null |
| **Priority** | Urgency, orthogonal to both | `low` · `medium` · `high` · `urgent` |
| **Claim** | Exclusive, expiring hold by one harness session | — |
| **Harness** | An AI coding client reading/writing the board | — |
| **Session** | One continuous run of a harness | caller-supplied |

**Status and triage are separate fields, not one longer list.** `ready-for-human` is not
"further along" than `ready-for-agent` — it is routed to a different consumer. Collapsing
them would destroy the ability to answer "what is a person staring at?", which is the
question a board with an agent in it most needs to answer.

---

## 6. Database Schema (`.seraph/state.db`)

### `tasks`

```sql
CREATE TABLE tasks (
    id            TEXT PRIMARY KEY,
    title         TEXT    NOT NULL,
    description   TEXT,
    status        TEXT    NOT NULL DEFAULT 'backlog'
                  CHECK (status IN ('backlog','todo','in_progress','review','done')),
    triage        TEXT
                  CHECK (triage IN ('needs-triage','needs-info','ready-for-agent',
                                    'ready-for-human','wontfix') OR triage IS NULL),
    priority      TEXT    NOT NULL DEFAULT 'medium'
                  CHECK (priority IN ('low','medium','high','urgent')),
    claim_harness TEXT,
    claim_session TEXT,
    claim_expires INTEGER,
    metadata      TEXT,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
```

Notes:

- **Enums are `CHECK`-constrained.** In v1.0 they were bare `TEXT`, so the documented value
  set was enforced by nothing but a comment.
- **Timestamps are `INTEGER` (Unix epoch seconds, UTC).** SQLite has no `DATETIME` type —
  `DATETIME` is a `NUMERIC` affinity, and RFC3339 strings stored in it do not compare
  correctly. Integers sort and range-query correctly with no conversion.
- **`claim_*` replaces v1.0's `assigned_harness` + `session_id`.** A separate "assigned to"
  field alongside a claim is two answers to one question. Assignment *is* the claim, and
  a claim without a harness, session, and expiry is not a claim.
- **`metadata`** is retained from v1.0 as a JSON escape hatch.
- **There is no claim token.** v2.0.0 specified one, and implementing it showed the
  problem: the harness would have needed the token to call `release_task`, and the only
  channel available is a snapshot that gets committed, where the token would be a
  published secret. It is also redundant — once a claim is taken over, `claim_session`
  names the new holder, which already refuses the previous session's release. The
  session *is* the authority.

### `id_sequence`

```sql
CREATE TABLE id_sequence (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    last INTEGER NOT NULL   -- most recently allocated number
);
```

Task IDs are allocated **in the same transaction as the insert**: `TASK-101`, `TASK-102`, …
IDs are never reused, including after deletion.

### Connection

```sql
PRAGMA journal_mode = WAL;   -- once, at bootstrap
PRAGMA foreign_keys = ON;    -- per connection
PRAGMA busy_timeout = 10000; -- per connection
```

WAL permits one writer alongside many readers across processes — the right shape when
several harnesses have Seraph running at once, which is the normal case rather than an edge
case. Two details are load-bearing and were both found by testing, not by reading:

- **`journal_mode` is set once, not per connection.** It is a persistent property of the
  file, and *switching* it needs a lock that `busy_timeout` does not cover. Asserting it
  on every pooled connection made a dozen harnesses starting at once race each other into
  `SQLITE_BUSY`, so some could not open the board at all. Processes now poll for `wal`; the
  first to switch wins and the rest observe it and proceed.
- **Transactions begin `IMMEDIATE`, not deferred.** Claiming is a read-then-write. Under a
  deferred transaction two harnesses can both read "unclaimed" and both write, and the
  loser is told nothing. This is verified: eight harnesses racing one task at the same
  instant produce exactly one winner, seven refusals, and one holder on the board.

`foreign_keys` and `busy_timeout` are genuine per-connection settings, so they travel in
the DSN and apply to every connection the pool opens.

### Relationship to the prior attempt

`~/.hermes/kanban.db` holds a 32-column agent-orchestration schema (workspace, branch, retry,
heartbeat, per-task model) with **zero rows**, wired into `~/.hermes/config.yaml`. Seraph is
greenfield and does not reuse or migrate it. Two columns from that schema are adopted
deliberately: **`claim_expires`**, which closes the crash-leaves-a-task-locked hole, and
`session_id` as part of the claim.

---

## 7. Render Engine

Rendering is **synchronous**. The v1.0 draft spawned a goroutine per mutation, which is a
silent data-loss bug: two concurrent mutations could each read the database, and the one
that read *older* state could `os.Rename` last, overwriting the newer board. `os.Rename` is
atomic per file; it provides no ordering between writers. Nothing errors — the board simply
regresses.

A render is one `SELECT` plus a few kilobytes of text. The cost is not measurable, and
synchronous rendering is what makes the consistency contract in §3.2 stateable at all.

**Contract:** the rendered board reflects every completed write, at the moment the write
returns.

1. **Query** — all tasks, ordered by status (`in_progress` → `review` → `todo` → `backlog` →
   `done`) then priority (`urgent` → `high` → `medium` → `low`).
2. **Render** — one canonical render produces both outputs, so they cannot disagree.
3. **Write** — each target is written to `.seraph/.seraph-*.tmp`, flushed, `fsync`ed, chmod
   `0644`, then `os.Rename`d over the target. Rename within a filesystem is atomic, so no
   external reader ever sees a partial file.

Both snapshots come from one render pass. `KANBAN.md` is the human view; `board.json` is
the same data for programmatic consumers. Neither is ever an input.

---

## 8. MCP Tool Interface

All tools take an explicit `session_id` on any mutating call. See §9.

| Tool | Purpose | Inputs | Notes |
| --- | --- | --- | --- |
| `get_board_summary` | Current board grouped by status | — | Returns the rendered view. Add `status_filter`, `include_done` |
| `get_task` | One task in full, including claim state | `task_id` | — |
| `create_task` | Create a task | `title`, `description?`, `priority?`, `triage?` | ID allocated in-transaction |
| `claim_task` | Take an exclusive, expiring hold | `task_id`, `harness_id`, `session_id` | Fails if live-claimed by another session; succeeds and takes over if expired |
| `update_task` | Change status, text, or triage | `task_id`, `session_id`, `status?`, `title?`, `description?`, `triage?`, `priority?` | **`status: done` rejected unless the caller holds the live claim** |
| `release_task` | Drop a claim you hold | `task_id`, `session_id` | Also implicit on transition to `done` |

`search_context` is **removed from v1** — it had no ingestion path. See §2 and §13.

### Error semantics

Errors are returned as **tool-level** results carrying a machine-readable `reason`, not as
protocol errors: a held claim is a normal event in a board several harnesses share, and
the caller is expected to read the reason and act on it. Genuine faults still surface as
protocol errors, where they belong.

`TASK_NOT_FOUND` · `CLAIM_HELD_BY_ANOTHER_SESSION` · `CLAIM_ALREADY_HELD` ·
`CLAIM_REQUIRED_FOR_DONE` · `NOT_CLAIMED` · `INVALID_TITLE` · `INVALID_STATUS` ·
`INVALID_TRIAGE` · `INVALID_PRIORITY`

Every claim-related refusal carries the current holder — harness, session, and expiry — so
a refused caller can tell a person who to talk to rather than merely that it failed.

---

## 9. Identity, Claims, and Sessions

**The protocol carries no ambient session identity.** `initialize` returns a client name and
version — the *harness* — and nothing more. There is no standard per-session identifier a
server can read.

So: **the caller names its session, on every mutating call.** A claim is the pair
(harness, session) plus an expiry, enforced against the explicit `session_id` on the call.

**The caller does not always get this right, and Seraph is built for that rather than
against it.** Two real harnesses racing for one task were observed inventing the *same*
session id, because two processes of one harness saw the same environment and each made
up an identifier. An earlier version let a re-claim by the same session succeed, so the
second process silently overwrote the first: the board showed a single holder and nothing
looked wrong, while both agents walked away believing the task was theirs. The failure was
invisible precisely because the board looked consistent.

So a **live claim is refused whoever is asking**, including its own holder:

- a different session asking → `CLAIM_HELD_BY_ANOTHER_SESSION`, with the holder named
- the same session asking → `CLAIM_ALREADY_HELD`, with the message saying so plainly, and
  naming the harness mismatch when the board records a different `harness_id` than the
  caller used — that mismatch is the signature of two callers sharing one session id

Refusing the holder costs nothing: any mutation by the holding session already refreshes
the expiry, so a caller that legitimately still holds the task has no reason to re-claim.
Verified against two real harnesses — the loser is refused, is told who holds it, and says
so.

- Claims carry a configurable expiry (default 30 minutes, override with `SERAPH_CLAIM_TTL`),
  refreshed by any mutation the holding session makes.
- An expired claim does not block a new `claim_task`; it is taken over silently, which is
  how a crashed session frees its work.
- Transitioning to `done` with a live claim releases it.
- A crashed session's tasks free themselves after expiry. `release_task` is for the tidy path.

This prevents two harnesses from colliding by accident. It does not defend against a caller
lying about its name, and is not intended to — this is a single-user local tool, and treating
it as adversarial would buy nothing. It only ensures that *two callers who agree on nothing
else* cannot both believe they hold the same task.

---

## 10. Install and Governance Injection

### What it writes

| Target | Purpose | Mode |
| --- | --- | --- |
| `.mcp.json` | Registers Seraph for harnesses that read project scope | Write / merge |
| `AGENTS.md` | Rules block, marker-delimited | Merge |
| `CLAUDE.md` | Same Rules block, marker-delimited | Merge |
| `.seraph/.gitignore` | Ignores local DB state while keeping snapshots committable (§4) | Create / overwrite |

`.mcp.json` is committable — Claude Code documents project-scope MCP servers as repository
files — so it is the right default and travels with the repository.

**The registered command is a bare binary name, never an absolute path:**

```json
{ "mcpServers": { "seraph": { "command": "seraph", "args": [] } } }
```

`.mcp.json` is committed and travels between machines, so a path to one developer's
`~/go/bin/seraph` is wrong for everyone else. This is the same convention `codegraph` uses
(`{"type":"stdio","command":"codegraph","args":["serve","--mcp"]}`) and the same one every
MCP config in a typical environment already follows.

### The AGENTS.md-only bug

v1.0 injected Rules into `AGENTS.md` alone. **That silently reaches no one in Claude Code
2.1.84**, which reads only `CLAUDE.md`; direct `AGENTS.md` reading requires v2.1.277+. Even
on a patched version, `CLAUDE.md` takes precedence when both exist unless the non-default
`claude-md-and-agents-md` setting is used.

So Seraph writes the same marker-delimited block to **both** files.

### Invariants

1. **Merge, never overwrite.** Content outside the markers is never touched. Re-running
   `install` updates the block in place and is idempotent.
2. **No file creation if absent** — a missing `AGENTS.md` or `CLAUDE.md` is created, since
   there is no user content to lose.
3. **Never rewrite a config Seraph does not understand.** Four live configs in a typical
   environment hold plaintext credentials (GitHub PAT, Atlassian token, Telegram bot and
   gateway tokens, database bearer). `install` refuses to touch a file whose structure it
   does not recognise, rather than risking a credential.
4. **Report every file, show the change, and make writing optional.** `install` prints each
   target with what happened to it — created, updated, or unchanged — followed by the lines
   that will change, `-` for what is removed and `+` for what replaces it, with the unchanged
   prefix and suffix elided. `--dry-run` produces the same report while writing nothing.
   There is no interactive prompt: `install` runs from wherever a harness or script launched
   it, and a blocking question with nobody at the keyboard is a hang.
5. **Refuse rather than guess.** A `.mcp.json` that exists but is not valid JSON, or whose
   `mcpServers` is not an object, is left untouched and install fails. An instruction file
   carrying one marker without the other — a state only hand-editing produces — is also
   left untouched. Both are cases where guessing wrong destroys the user's text.
6. **Converge.** Running `install` any number of times leaves every file byte-identical to
   the previous run. A file that grows on every run is a file being clobbered, quietly,
   on a repository nobody asked us to touch.
7. **The registered command is PATH-resolved.** See the `.mcp.json` shape above.

### The Rules block

```markdown
<!-- SERAPH RULES START -->
## Working through the Seraph board

This project tracks its work with the `seraph` MCP server, which is the source of truth.
`.seraph/KANBAN.md` and `.seraph/board.json` are rendered from it — never edit them
directly; your edits are overwritten on the next change.

1. Before starting work, call `claim_task` with this session's `session_id`. A task already
   claimed by another session is not yours to take — surface it rather than taking it.
2. Move the task with `update_task` as you go. The server refuses to mark a task `done`
   unless your session holds its claim, so work that was never claimed cannot be completed.
3. When finished, call `release_task` so the next harness can pick the task up.
4. Call `get_board_summary` before creating tasks, so you do not duplicate tracked work.
<!-- SERAPH RULES END -->
```

The Rules are **advisory**. They tell a cooperating harness how to behave; §8's tool layer is
what actually holds the line. A harness that never read them still cannot mark unclaimed
work done.

---

## 11. CLI Surface

| Command | Purpose |
| --- | --- |
| `seraph install [--dry-run]` | Register Seraph, place the Rules, and report every file as created, updated, or unchanged |
| `seraph doctor` | Report repository, database, snapshot, and per-harness registration health |
| `seraph version` | Print the version |
| `seraph` (no args) | Run the MCP server on stdio |

`SERAPH_CLAIM_TTL` sets how long a claim survives, e.g. `45m` (default 30m).

`doctor` is not optional. For a server with no daemon and no logs, it is the only way a user
diagnoses "the board is empty" — the single most common first failure. It reports
repo-root resolution, database reachability, journal mode, applied schema version, task
count, snapshot state, and every detected harness with its config path and key.

**Snapshot state is decided by comparing content, not timestamps.** Under WAL, writes land
in `state.db-wal`, and a clean shutdown checkpoints that log back into `state.db` *after*
the snapshot was written — so a snapshot that is exactly right looks permanently stale by
mtime. The shared-memory index is worse: it is touched on every connection open and close.
Rather than guess among those, `doctor` renders the board and diffs the bytes, and reports
`current` or `DIFFERS`. `doctor` never writes; repairing the snapshot is a tool call's job.

---

## 12. Dependencies (pinned)

| Module | Version | Why |
| --- | --- | --- |
| `modernc.org/sqlite` | v1.60.1 | Pure Go. FTS5, RTREE, and JSON1 compiled in unconditionally. Cross-compiles from any host. |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | Official MCP SDK, v1 stable. |

**`github.com/mattn/go-sqlite3` is rejected** — see [ADR-0002](./docs/adr/0002-pure-go-sqlite-driver.md).
Its FTS5 is behind a `sqlite_fts5` build tag, and `go install` accepts no build tags, so
every executable installed by anyone other than the author silently lacks it and fails at
runtime. It also requires cgo, which means no cross-compiled macOS build from Linux.

**"Zero-dependency" is removed from this document.** It was never true. The accurate claim is:
*no daemon, no container, no runtime service, one static binary.*

Known cost: `modernc.org/sqlite` is a large dependency tree, and its source lives on a forge
outside the primary one. Accepted, and recorded in ADR-0002.

---

## 13. v2: The Index

The Index is what makes Seraph a universal context layer rather than a unified board. It is
deferred **whole** — no table, no tool, no stub — because a reader without a writer returns
nothing, forever.

**Its corpus is settled: prose only.** Code intelligence belongs to `codegraph`, permanently
— see [ADR-0004](./docs/adr/0004-seraph-indexes-prose-not-code.md) for the evidence, which
goes well past "somebody already built it." Every question below is therefore about Markdown,
not about source.

Designing it requires answering, in order:

1. **Scope** — which Markdown, which roots, which exclusions. Start from what a project
   already emits: `CONTEXT.md`, `docs/adr/`, `.scratch/`, design notes. Exclude `.git`,
   `node_modules`, lockfiles, and generated output. Honour `.gitignore` root and nested,
   following the `codegraph` precedent.
2. **Unit** — file, heading section, or fixed-size chunk. This drives result quality more
   than the search itself.
3. **Ingestion trigger** — on `install`, on an explicit `reindex` tool, on file change, or on
   demand at query time.
4. **Staleness** — content hash, not mtime. A branch checkout or fresh clone rewrites mtimes
   across the whole tree, so an mtime-based index reports everything stale exactly when
   several harnesses are working. `codegraph` already uses `files.content_hash`; copy that.
   A stale index returns older results *and* a warning naming the stale files — never a
   silent answer, never a self-repair.
5. **Concurrency** — two harnesses reindexing the same repository simultaneously.
6. **Naming** — `search_context` was wrong on this vocabulary. It returns results from the
   **Index**, not from the **Context**. It should be `search_docs` or `index_search`.

FTS5 is already available in the chosen driver, and must not be used for source code
(ADR-0004), so this is a design problem about prose rather than a capability problem.

---

## 14. Changes from v1.0.0

Fixed, with the defect that prompted each:

| v1.0 § | Defect | Fixed in v2.0 |
| --- | --- | --- |
| 1 | Claimed "zero-dependency" — untrue in both directions | Removed; accurate claim substituted (§12) |
| 2.1 | "SQLite as Single Source of Truth" — but every consumer reads files, so two readers with no agreement | The rendered board is the source of truth (§3.1) |
| 2.4 | Rules injected into `AGENTS.md` only — silently ignored by Claude Code 2.1.84 | Injected into both `AGENTS.md` and `CLAUDE.md` (§10) |
| 3 | `.context/` held board state; "context" meant three different things | `.seraph/`; vocabulary fixed in [CONTEXT.md](./CONTEXT.md) |
| 4 | `status`/`priority` were bare `TEXT` — enums enforced by a comment | `CHECK` constraints (§6) |
| 4 | No claim expiry, no release path — a crashed session locked tasks forever | `claim_expires` and `release_task` (§6, §9) |
| 4 | `task_events` justified by an undesigned "session playback" | Removed (§2) |
| 4 | `docs_fts` with no writer; `search_context` could never return a row | Deferred to v2, whole (§13) |
| 4 | `created_at` / `updated_at**` — broken markdown; `DATETIME` affinity breaks comparison | Integer epoch; markup fixed (§6) |
| 5 | **Async exporter raced — two writers, no ordering, silent board regression** | Synchronous; contract stated (§7) |
| 6 | No `release_task`, no `get_task` | Added (§8) |
| 6 | `done` required no claim — a model could mark unfinished work complete | Enforced in the tool layer (§8, ADR-0003) |
| 6 | Errors unspecified | Machine-readable reasons with holder payloads (§8) |
| 7 | `install` would have merged into files holding live credentials, unspec'd | Never-clobber invariants (§10) |
| 7 | Only `install` — no way to diagnose anything | `doctor` added (§11) |
| 7 | "atomic atomic" (typo); ordering unspecified | §7 |
| 4, 6 | Index scope left implicit, in a codebase that already ships a code index | Corpus fixed to prose; boundary recorded (ADR-0004) |
| 3 | No commit policy for the state directory — DB and snapshots have different fates | Split policy, self-written `.seraph/.gitignore` (§4, §10) |
| 2.4, 7 | Registered command unspecified; an absolute path breaks on every other machine | PATH-resolved bare binary, invariant 5 (§10) |
| 6 | v2.0.0 specified a `claim_token` for release; implementing it showed the token would have to reach the harness through a *committed* snapshot, and was redundant with `claim_session` anyway | Token removed; the session is the authority (§6, §9) |
| 6 | Two reasons described one event — `TASK_ALREADY_CLAIMED` and `CLAIM_HELD_BY_ANOTHER_SESSION` | Merged into `CLAIM_HELD_BY_ANOTHER_SESSION` (§8) |

Four decisions are recorded as ADRs because they are hard to reverse, surprising to a
reader, and the result of a real trade-off:

- [ADR-0001](./docs/adr/0001-rendered-view-is-the-product.md) — the rendered board, not the database, is what consumers read
- [ADR-0002](./docs/adr/0002-pure-go-sqlite-driver.md) — pure-Go SQLite driver over cgo
- [ADR-0003](./docs/adr/0003-completion-requires-a-claim.md) — a task is finished only by the session holding its claim
- [ADR-0004](./docs/adr/0004-seraph-indexes-prose-not-code.md) — Seraph indexes prose; code intelligence is not duplicated
