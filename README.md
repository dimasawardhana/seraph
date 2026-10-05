# seraph

A local task board for AI coding harnesses. One board per repository, stored in a SQLite
file beside the code, served over MCP. Harnesses read it and write to it instead of keeping
their own state, so what a project decides outlives the session that decided it.

It is one file, no server, no account, and no network. `go build` and it runs.

```bash
git clone https://github.com/dimasawardhana/seraph && cd seraph
go build -o ~/go/bin/seraph .
~/go/bin/seraph doctor          # creates .seraph/state.db and reports what it found
cd ~/code/yourproject
seraph install                   # register with your harnesses, add the Rules block
seraph board                     # or just look at the board in your terminal
```

## What a harness gets

Six MCP tools: `get_board_summary`, `get_task`, `create_task`, `claim_task`, `update_task`,
`release_task`. A harness claims a task before touching it, and **only the session holding a
claim can complete it** — that is the single rule the server enforces. Two harnesses working
at once cannot both believe a task is theirs.

A task carries a title, a goal and acceptance criteria, plus a description, a status, a
priority, and triage saying who could pick it up. Claims expire on their own (30 minutes
by default, `SERAPH_CLAIM_TTL` to change it), so an agent that dies mid-task does not hold it
forever.

## The claim gate

A harness that can run a pre-tool hook can also have its writes *refused* when the session
holds no claim. `seraph install` wires the gate into the harnesses that support one and reports
the rest as advisory.

`seraph hook` tells you which, and it is careful with the word "enforcing":

| State | Meaning |
| --- | --- |
| `enforcing (verified <build>, <date>)` | somebody watched this harness refuse a real write |
| `installed, unverified` | the hook file is there; **nobody has confirmed the harness reads it** |
| `cannot enforce` | the harness has no hook surface that can block |
| `not installed` | no hook |

Today exactly one harness holds a verified entry. The rest are `installed, unverified`
because a file being present is not evidence of anything, and this project has been bitten by
that distinction twice already. `seraph hook --verify` re-checks a recorded entry against the
build actually installed, so an upgrade cannot quietly keep a claim nobody re-earned.

## What lives where

| Path | Committed? | What it is |
| --- | --- | --- |
| `.seraph/state.db` | no — machine-local | the board |
| `.seraph/KANBAN.md` | yes | the board, rendered for humans |
| `.seraph/board.json` | yes | the same data, for scripts |
| `.mcp.json`, `AGENTS.md`, `CLAUDE.md` | yes | how a teammate's harness gets set up |

Cloning a repository that already has a board is the case this got wrong first: you get the
history in the two snapshots and no database, and the first tool call would render an empty
board over the top of it. Seraph now seeds an empty board from the snapshot when it opens one,
and never touches a board that already holds a task.

## What it is not

No accounts, no sync, no remote, no multi-user. One board per repository. No event log —
changing a status overwrites it. No document search, and no task CLI: you move work through a
harness, not by hand.

There is more in the roadmap than here. None of it is built, and this file will not pretend
otherwise. [ROADMAP.md](./ROADMAP.md) says what was decided and what was deferred, with the
reasons; [prd.md](./prd.md) is the specification the code is written against.

## Reading further

- **[docs/USING.md](./docs/USING.md)** — the full manual: every command, the gate's exact
  behaviour, what to commit, and the harnesses that can enforce it
- **[CONTEXT.md](./CONTEXT.md)** — the vocabulary, and what each word is deliberately not
- **[DESIGN.md](./DESIGN.md)** — why it is built this way
- **[ROADMAP.md](./ROADMAP.md)** — what is done, what is carried forward, what is deferred
- **[docs/adr/](./docs/adr/)** — the decisions that shaped it, and what each one cost
