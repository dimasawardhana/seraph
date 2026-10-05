# Using Seraph

Seraph gives your AI coding harnesses a shared task board that survives the session. Install it
into a repository once; after that every harness on that repo reads and writes the same board, and
no two of them start the same work.

You mostly don't call it directly — your harness does, through its tools. When you want to look
yourself, there are two dashboards: one in the terminal, one in a browser.

**Contents**

- [Install](#install) · [Verify](#verify) · [First use](#first-use)
- [Dashboards: seeing and doing it yourself](#dashboards-seeing-and-doing-it-yourself)
- [The six tools](#the-six-tools) · [The one enforced rule](#the-one-enforced-rule)
- [The claim gate](#the-claim-gate-refusing-a-write-nobody-claimed)
- [Two harnesses, one task](#two-harnesses-one-task)
- [Reading the board yourself](#reading-the-board-yourself) · [Values](#values)
- [Configuration](#configuration) · [What to commit](#what-to-commit)
- [Adding other harnesses](#adding-other-harnesses)
- [Limits of v1](#limits-of-v1) · [Troubleshooting](#troubleshooting)

---

## Install

### 1. Get the binary onto your PATH

```bash
cd ~/Documents/code/seraph
go build -o ~/go/bin/seraph .
which seraph          # must print a path
```

The `which` step is not optional. Seraph registers itself under the bare command `seraph`, so if
it isn't on PATH a harness will fail to spawn it with an error that looks like a config problem.

### 2. Install it into a repository

```bash
cd ~/Documents/code/myproject
seraph install --dry-run     # see what it would do, write nothing
seraph install               # do it
```

Output:

```
seraph install

  created   .mcp.json
            registered "seraph"; 0 other server(s) preserved
            + {
            +   "mcpServers": {
            +     "seraph": {
            +       "args": [],
            +       "command": "seraph"
            +     }
            +   }
            + }

  created   .seraph/.gitignore
            + state.db
            + state.db-wal
            + state.db-shm

  created   AGENTS.md          Rules block, 854 bytes
  created   CLAUDE.md          Rules block, 854 bytes
```

Four files, and it prints the lines it is adding to each. Re-running is safe: a second `install`
reports every file `unchanged` and writes nothing.

**What it will never do.** Overwrite text outside its own marker block. Touch a config file it
doesn't fully understand (several harness configs hold live credentials). Create a file that isn't
one of the four above.

If `AGENTS.md` or `CLAUDE.md` already exist, your content is preserved byte-for-byte and the Rules
block is appended below it.

## Verify

```bash
seraph doctor
```

```
repository   /home/you/myproject
marker       .git
database     /home/you/myproject/.seraph/state.db
journal      wal
schema       v1 (this build expects v1)
tasks        0
KANBAN.md    absent — any tool call writes it
board.json   absent — any tool call writes it

harnesses
  project scope — written by `seraph install`
    registered MCP config            .mcp.json          key
    registered Rules (AGENTS.md)     AGENTS.md          key
    registered Rules (CLAUDE.md)     CLAUDE.md          key
  user scope — reported, never edited (several hold live credentials)
    present, no seraph entry opencode         ~/.config/opencode/opencode.json  key mcp
    present, no seraph entry Claude Code      ~/.claude.json                    key mcpServers
    ...
```

How to read it:

- **`repository` / `marker`** — `marker none found` means there is no `.git` or `.seraph/` above
  you and Seraph assumed your current directory. Usually means it ran somewhere unexpected.
- **`KANBAN.md absent`** — normal before first use. Any tool call writes it.
- **project scope `registered`** — install worked.
- **user scope `present, no seraph entry`** — expected in v1. Seraph deliberately does not edit
  those files; see [Adding other harnesses](#adding-other-harnesses).
- **`DIFFERS from the board`** — something changed the database out of band. Any tool call
  rewrites the snapshots.

`doctor` never writes anything. It exits non-zero only when something is actually wrong.

## First use

Open the repo in your harness and ask something ordinary:

> What work is tracked for this project, and is anyone already on it?

It should find the board on its own, from the Rules block in `AGENTS.md` / `CLAUDE.md`, and answer
from it. Nothing further is needed — that is the entire setup.

Then add work:

> Add a task to retire the nightly exporter cron. High priority, ready for an agent.

---

## Dashboards: seeing and doing it yourself

Your harness can read and write the board, but sometimes you want to look at it — or take a task
yourself. There are two ways, and both write through the same rules your agents do, so the claim
rule still holds.

### Terminal

```bash
cd ~/Documents/code/myproject
seraph board
```

```
  IN PROGRESS 1
▸  ● TASK-101  !        Replace the legacy exporter              claude-code · 24m
   BACKLOG 2
   ● TASK-103  ↑        Migrate the auth middleware                cursor · 24m
   ○ TASK-102  low      Write the runbook                            clear
  DONE 1
   ○ TASK-104  medium   Prove the claim holds                        clear
  j k move · c claim · s start · r review · d done · u release · n new · q quit
```

| Key | Does |
| --- | --- |
| `j` `k` / arrows | move the selection |
| `g` `G` | jump to first / last |
| `c` | claim the selected task — you become its holder |
| `s` | move to in progress |
| `r` | move to review |
| `d` | complete it and release |
| `u` | release without completing |
| `n` | new task — type a title, `esc` to cancel |
| `q` | quit |

It refreshes every second, repainting in place rather than clearing — no flicker beside a
terminal that is doing its own thing. In a narrow window it drops the priority column and
then the clearing time, rather than wrapping a row and pushing the board off screen.

**Reading a row:**

```
  ●  ○    held, or clear, or yours (the ringed dot is yours)
  TASK-101 the task id — red when urgent, amber when high
  !  ↑  ! and ↑ mark urgent and high; other priorities are spelled out
  the title
  claude-code · 24m  who holds it, and when it clears
```

A claim clearing within five minutes shows amber — that is the window where acting late
changes the outcome.

### Browser

```bash
seraph ui              # default port 7777
seraph ui --port 8080
```

```
seraph board at http://127.0.0.1:7777/?t=0fe2adbbf303644ebe3e5d4a420a265c
bound to 127.0.0.1 — the token in that URL is required for every request
```

Open that URL. The same panel, one row per task, click or keyboard:

- every key above works identically — `j`/`k` to move, `c` to claim, `d` to complete
- `/` filters by title, id, or holder, and counts the matches
- `↵` opens a task's description in place, under its row
- the page re-renders only when the board actually changes, so typing, hovering and
  reading survive every refresh

**Two deliberate restrictions, both about safety:**

- **It listens on `127.0.0.1` only** — not reachable from your network.
- **Every request needs the token in that URL.** A page at localhost that accepts form
  posts is reachable by any website you happen to have open; without the token, a stray
  page could claim or complete your tasks. The token is minted fresh each run, so closing
  the server invalidates it.

Consequences worth knowing: the token is in your browser history, and anyone with access to
your machine and the URL can act. That is the same trust level as your shell — if you need
less, use `seraph board`, which has no network surface at all.

### What the dashboards enforce

They are not a bypass. A human claim is a claim: it has an expiry, it can be released, and it can
be taken over once it lapses. And you **cannot** complete a task an agent is holding — you get the
same refusal the agent would:

```
CLAIM_REQUIRED_FOR_DONE — TASK-101 can only be completed by the session holding its claim
```

If you want to finish work an agent abandoned, wait out its claim or use its harness to release it.
Changing the status of an unclaimed task, or any task you hold, is always allowed.

---

## The six tools

Your harness calls these. Knowing them tells you what to ask for and what went wrong.

| Tool | Inputs | What it does |
| --- | --- | --- |
| `get_board_summary` | — | The whole board, grouped by status, in reading order |
| `get_task` | `task_id` | One task in full, as JSON, including its claim |
| `create_task` | `title`, `goal`, `acceptance`, `description?`, `priority?`, `triage?` | Adds a task. Starts in `backlog`. All three of `title`, `goal` and `acceptance` are required — the server refuses a task that records only *what* and cannot say whether it was the right work |
| `claim_task` | `task_id`, `harness_id`, `session_id` | Takes an exclusive, expiring hold |
| `update_task` | `task_id`, `session_id`, `status?`, `title?`, `goal?`, `description?`, `acceptance?`, `priority?`, `triage?` | Changes a task |
| `release_task` | `task_id`, `session_id` | Gives up a claim without completing |

**Every tool returns the whole board.** A harness never needs a second call to see current state,
and a file you open can never disagree with what a harness was just told.

The normal loop:

1. `get_board_summary` — look before you add, so you do not duplicate tracked work
2. `create_task` — if it is genuinely new
3. `claim_task` — when work actually starts
4. `update_task` — as it moves along
5. `update_task` to `done` — which also releases the claim

## The one enforced rule

**A task can only be completed by the session holding its claim.** Everything else is open: you can
move a task to `in_progress` without claiming it, create work freely, and change anything you like.

This exists so the board can answer *who did this, and when*. A board full of tasks nobody claimed
isn't worth keeping.

What it looks like when a harness tries to skip the step:

```
### update_task status=done, with no claim
    CLAIM_REQUIRED_FOR_DONE
    TASK-101 can only be completed by the session holding its claim; call claim_task first
```

The rule is enforced by the server, not by the Rules text. A harness that never read `AGENTS.md`
still cannot complete unclaimed work.

---

## The claim gate: refusing a write nobody claimed

The server refuses `done` without a claim. The **gate** is the other half: it refuses the *write*
itself, before the tool runs, so a harness that skips the board cannot quietly edit files and leave
the board describing work that never happened.

It is a separate command because hooks execute a command, and that command has to be fast, must not
open a database a parent process already has open, and must not depend on the harness having loaded
Seraph as an MCP server.

### seraph gate

The pre-tool hook a harness runs. You almost never call it by hand — `seraph install` wires it into
every harness that can enforce one. It reads a JSON payload on stdin, answers on stdout, and exits
`0` to allow or `2` to block:

```bash
seraph gate --harness claude-code
```

Allowed, because that session holds a task:

```
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":""}}
seraph: holding TASK-104
```

Refused, because that session holds nothing:

```
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"seraph: this is a write tool and this session holds no task. Record the work first: create_task with a title, a goal, and acceptance criteria, then claim_task. 1 task(s) are unclaimed on this board."}}
```
exit status 2

Four things worth knowing before you rely on it:

- **Reading is never gated.** `read`, `grep`, `glob` and the rest pass with no claim at all. A
  harness is entitled to look before it leaps.
- **The shell is gated too, deliberately.** A gate matching only `Write` could be walked around
  with `bash -c 'echo x > file'`, leaving the board lying about what was done. The cost is that shell
  use now needs a live claim as well.
- **An unreadable board allows; a missing one refuses.** If `state.db` exists but cannot be opened —
  corrupt, locked, wrong permissions — the gate lets the write through rather than freezing your
  work; a broken board should not be able to lock you out of your repo. A database that is simply
  *absent* is not an error, though: it reads as a board with nothing on it, and with no task to point
  at, the gate refuses. A freshly installed repository therefore refuses writes until it has a task.
  `create_task` is exempt from the gate, so that is how you get unstuck.
- **It usually cannot tell who is asking.** The gate reads `session_id` from the payload when a
  harness supplies one — this project's own generated omp hook passes `SERAPH_SESSION_ID` — but most
  do not. With no session the gate allows only when someone holds a claim *and* nothing is unclaimed;
  if any task is sitting unclaimed it denies, which routes the agent to the board instead of past it.
  A wrong guess here would be a deadlock, which is why the no-session case is this narrow.

Seraph's own MCP tools are never gated. Otherwise claiming would require creating first, and every
write would stay locked behind the mechanism meant to unlock them.

### seraph hook

Reports which harnesses can enforce the gate and which cannot, **without writing anything**, so it is
safe to run at any time:

```bash
seraph hook            # --explain shows the shape each harness expects; --verify checks the entries
```

```
  installed, unverified  Claude Code    ~/Documents/code/myproject/.claude/settings.json
  installed, unverified  Codex          ~/.codex/hooks.json
  not installed          Cline          ~/.cline/plugins/seraph-gate/plugin.json
  cannot enforce         Kiro           ~/Documents/code/myproject/.kiro/hooks/seraph-gate.json
                         kiro-cli 2.x has no hook surface; V3 announces hooks whose shape is
                         not established here; a gate file written by an earlier install is
                         still at that path and is inert
  cannot enforce         Zed            ~/.config/zed/settings.json
                         no agent hook surface; use agent.tool_permissions.tools.edit_file.always_deny
  cannot enforce         opencode       ~/.config/opencode/opencode.json
                         its pre-tool hook can only mutate arguments; use permission.edit: "deny"
```

Four states, and the difference matters:

| State | Meaning |
| --- | --- |
| `enforcing (verified <build>, <date>)` | Somebody watched this harness refuse a real write, and wrote down which build they watched |
| `installed, unverified` | Seraph wrote the hook and it is still there. **Nothing has confirmed the harness reads it** |
| `not installed` | No hook — the Rules still apply, but nothing refuses a write |
| `cannot enforce` | The harness has no hook surface that can block. The lever is a permission setting, not a hook |

**One harness reports as `enforcing` today: omp, verified on omp 18.0.3, 2026-10-05.** A
subprocess was told to run `echo gateprobe > /tmp/probe.txt`, the hook refused the shell call
with the gate's own message, and the file was confirmed absent afterwards. Every other harness
with a hook file reads `installed, unverified`, and that is the honest reading rather than a gap
in the report.

It is tempting to read `installed` as coverage; it is not evidence of any, and the distance
between the two is not theoretical. kiro-cli 2.11.1 shipped a hook file, `seraph hook` reported
it installed, and a write from a kiro session holding no claim went straight through — the
binary's only `PreToolUse` string is its own changelog entry for the feature request that would
add one. omp had the same disease for longer: seraph wrote its gate to `.omp/hooks/pre/`, a
directory omp does not discover, and shaped it with the wrong API type. It reported installed for
days and never fired once.

Two claims worth keeping apart, because only the first has been demonstrated everywhere:
**`seraph gate` refuses a payload**, and **a given harness invokes it**. Every payload test in
this project's history fed `seraph gate` directly. That proves the gate. Whether Claude Code,
Cursor, Codex, Copilot CLI or Gemini CLI ever call it is a separate question, asked per harness,
per build — and omp is currently the only one with an answer.

To earn the first row, watch a harness refuse a real write on your own machine and record what
you saw. Do not add the entry from documentation — and do not let it go stale. `seraph hook
--verify` runs each verified harness's own version command and compares it against the build
the entry was witnessed on:

```
  holds                  omp                    recorded omp 18.0.3, installed reports "omp/18.0.3"
```

`holds` means the build still matches, not that the refusal still holds: re-watching one is a
person's job. An upgrade that changes the version prints `STALE` instead, so a claim nobody
re-earned cannot quietly outlive the build behind it.

`seraph install` is what wires them, and it refuses to guess: a config whose structure it does not
recognise is left alone and reported rather than rewritten. Install reports what it *did* —
`installed`, `would install`, `unchanged`, `refused`, `advisory only` — while `seraph hook`
reports what *enforces*. An action is not a claim, and the two are not the same sentence.

> Several of these files hold live credentials for other servers. Seraph extends them rather than
> rewriting them — a config whose structure it does not recognise is left alone and reported. It does
> write user-scope *hook* files for the harnesses it can enforce on, since three of the targets above
> (Codex, Copilot CLI, Gemini CLI) live in your home directory. What it never touches is user-scope
> *MCP registration* — adding the server entry there stays your job, and the shapes follow.

Run `seraph install --dry-run` first to see the diff without writing anything.

## Two harnesses, one task

The second harness is refused and **told who holds it**, so it can report rather than work around:

```
### claim_task by a second harness
    CLAIM_HELD_BY_ANOTHER_SESSION
    holder: {'harness': 'claude-code', 'session': 'sess-c', 'expires': 1790812990}
```

A well-behaved agent says *"I do not hold TASK-101 — claude-code has it until 00:03."* That is the
whole point: the answer routes to you instead of two agents colliding.

**Claims expire** — 30 minutes by default, refreshed every time the holder updates the task. A
crashed session therefore frees its work rather than locking a task forever. If an agent died
mid-task, wait out the TTL rather than fighting it.

A harness that attempts to claim a task **it already holds** gets `CLAIM_ALREADY_HELD` rather than
a silent refresh. It does not need to re-claim: any update it makes extends the claim.

## Reading the board yourself

You never need a harness to see the board. Open the file:

```markdown
# Board

_Written by Seraph. Do not edit — this file is replaced on every change._

## In Progress

### TASK-101 · Replace the legacy exporter

- priority: high
- triage: ready-for-agent
- claimed by: claude-code (session `sess-c`, until 2026-10-01T00:03:10Z)
- updated: 2026-09-30T23:33:10Z

> Retire the nightly cron that rewrites KANBAN.md.
```

Two files, both regenerated on every change:

- **`.seraph/KANBAN.md`** — the view above, for humans
- **`.seraph/board.json`** — the same data, for scripts

**Never edit either one.** They are outputs. Any edit is replaced on the next change, silently. To
change something, ask your harness.

## Values

| Field | Values |
| --- | --- |
| `status` | `backlog` · `todo` · `in_progress` · `review` · `done` |
| `priority` | `low` · `medium` · `high` · `urgent` |
| `triage` | `needs-triage` · `needs-info` · `ready-for-agent` · `ready-for-human` · `wontfix` — or none |

**Status and triage answer different questions.** Status is where a task is. Triage is *who can
pick it up* — `ready-for-human` is not "further along" than `ready-for-agent`, it is routed to a
different consumer. Keeping them separate is what lets you still ask "what is a person staring
at?" on a board with agents on it.

A task with no triage is simply not triaged yet. That is normal.

### References

Every task can name the documents that explain it. They go on `create_task` and
`update_task` as a list of repository-relative paths, each optionally carrying an anchor:

```json
{"task_id": "TASK-119", "references": ["CONTEXT.md#index", "docs/adr/0001-rendered-view-is-the-product.md"]}
```

They render under the task in `KANBAN.md`, and they are carried in `board.json` so a clone
gets them back with the board.

**Seraph does not hold these documents.** They stay in the repository, where git can review
them — putting them in the database would make every one of them machine-local, and they
would be gone from the next clone.

Two things follow from that, and both are deliberate:

- A reference must be relative to the repository root and must not climb out of it. An
  absolute path means one thing to whoever wrote it and nothing to anyone else, so it is
  refused with `INVALID_REFERENCE` rather than stored.
- **A reference whose file is missing is reported by `seraph doctor`, not written into
  `KANBAN.md`.** That file is a pure function of the database — the same board must render
  identically on every machine — so anything that reads the filesystem stays out of it.

This is not the Index. `prd.md` §13 designs a searchable projection of a project's prose;
that is a larger thing, still deferred. A reference is a path today and can resolve to an
index entry later without this shape changing.

## Configuration

```bash
SERAPH_CLAIM_TTL=2h seraph     # how long a claim survives; default 30m
```

In a harness config, set it as an environment variable on the server entry.

## seraph version

```bash
seraph version
```

```
0.1.0-dev
```

The same string is sent in the MCP handshake, which is how you tell which build a harness is actually
talking to — worth checking when two harnesses disagree about what they see. It is a plain constant
in `internal/version`.

## What to commit

| File | Commit? |
| --- | --- |
| `.seraph/state.db` (+ `-wal`, `-shm`) | **No** — machine-local, already gitignored |
| `.seraph/KANBAN.md`, `.seraph/board.json` | **Yes** — they are the human record |
| `.mcp.json`, `AGENTS.md`, `CLAUDE.md` | **Yes** — that is how a teammate gets set up |
| `.seraph/.gitignore` | **Yes** |

`.seraph/.gitignore` is self-contained: install writes it rather than touching your root
`.gitignore`.

**Cloning a repository that already has a board.** The database is deliberately not committed
and the snapshots are, so a fresh clone arrives holding the whole history in two files and no
database — and the first tool call would otherwise render an empty board over the top of it.
Seraph restores it instead: the first command to open a board seeds an empty one from
`board.json` and says so —

```
board restored 16 task(s) from the committed board.json
```

It seeds **only** a board holding zero tasks, so a board you have worked on is never replaced by
an older snapshot, and a second command on the same clone reports nothing because there is
nothing left to restore. If the snapshot contains anything this build does not understand — an
unknown status, say — the whole seed is refused rather than half-applied, and `seraph serve`
refuses to start rather than hand you an empty board it would then overwrite.

## Adding other harnesses

v1 writes the project-scope config only. For the rest, add a server entry yourself. **The shapes
genuinely differ** — copy the right one.

### Harnesses using `mcpServers` with a string command

Claude Code (`~/.claude.json`), Cursor (`~/.cursor/mcp.json`), Cline (`~/.cline/mcp.json`),
Kiro (`~/.kiro/settings/mcp.json`), Copilot CLI (`~/.copilot/mcp-config.json`),
omp (`~/.omp/agent/mcp.json`), Antigravity (`~/.gemini/antigravity/mcp_config.json`):

```json
{
  "mcpServers": {
    "seraph": { "command": "seraph", "args": [] }
  }
}
```

### opencode — different shape

`~/.config/opencode/opencode.json`, key `mcp`. Note `command` is an **array**, and environment
variables go under `environment`, not `env`:

```json
{
  "mcp": {
    "seraph": { "type": "local", "command": ["seraph"], "enabled": true }
  }
}
```

### Zed — different shape

`~/.config/zed/settings.json`, key `context_servers`, command is a string again:

```json
{
  "context_servers": {
    "seraph": { "command": "seraph", "args": [] }
  }
}
```

### hermes — YAML

`~/.hermes/config.yaml`, key `mcp_servers`:

```yaml
mcp_servers:
  seraph:
    command: "seraph"
    args: []
```

### openclaw — nested

`~/.openclaw/openclaw.json`, key `mcp.servers`:

```json
{
  "mcp": {
    "servers": {
      "seraph": { "command": "seraph", "args": [] }
    }
  }
}
```

*Shape taken from openclaw's own documentation, not verified against a working config — the
machine this was written on has no `mcp` key in that file yet.*

After adding one, run `seraph doctor` — that entry should read `registered`.

> Several of these files hold live credentials for other servers. Edit them by hand and carefully;
> Seraph will not touch them for exactly that reason.

## Limits of v1

Know these before relying on it:

- **No task CLI.** You add and move work through a harness, not `seraph add`. The only thing you
  can do by hand is read the snapshots.
- **No search.** Document indexing is deliberately out of v1 — `grep` is still your tool.
- **No history.** Changing a status overwrites it; there is no event log.
- **One board per repository**, resolved by walking up to the nearest `.git` or `.seraph/`.
- **Harness instruction files are advisory.** Enforcement lives in the tools, which is why the
  completion rule holds even for a harness that ignores `AGENTS.md`.

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| `marker none found` in doctor | No `.git` or `.seraph/` above your working directory |
| Harness shows no seraph tools | `which seraph` fails, or the config has not been picked up. Many harnesses reload without a restart — omp takes `/mcp reload`, which re-reads `mcp.json` from disk |
| Harness can't spawn the server | `command` must be the bare name `seraph`, not a path — and it must be on PATH |
| Snapshot says `DIFFERS from the board` | Either something edited the database or the file out of band, or you are on a fresh clone where the database does not exist yet. `seraph doctor` tells you which: it restores an empty board from the snapshot and then reports both as current |
| Agent says it finished but the board disagrees | Trust `get_task`; the claim state is authoritative, not what the agent said |
| Task stuck claimed by a dead agent | Wait out the TTL. Only the session that took the claim can release it — `release_task` from any other is refused — and a shorter `SERAPH_CLAIM_TTL` set afterwards changes nothing already written. Once the claim lapses, another session can claim the task |
| `CLAIM_ALREADY_HELD` on a claim | The calling session already holds it — it should just keep working |
| `install` refuses a `.mcp.json` | That file exists but is not valid JSON. Fix it; install will not guess |
