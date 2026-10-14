# Board

_Written by Seraph. Do not edit — this file is replaced on every change._

## Backlog

### TASK-133 · The gate classifies tools by name, so any write-capable tool it does not name runs unclaimed

> **Goal:** Evaluate decides by matching a tool name against a list of write verbs and exec verbs; a harness whose write tool is named something else — eval, task, hub — is allowed to modify files while holding no claim, and an omp session was observed doing exactly that, so the gate must refuse by capability rather than by a vocabulary it cannot keep up with.
- priority: high
- triage: ready-for-agent
- updated: 2026-10-14T04:09:51Z
references:
  - internal/gate/gate.go

> **Done when:** VERIFIED 2026-10-14, this repository, installed binary ~/go/bin/seraph rebuilt from ac8c3c9, 2 tasks unclaimed on the board, this session holding no claim.
> **Done when:** THE DECISIVE TEST — same destination, same session, two tools. The strongest form this can take, because it removes every alternative explanation:
> **Done when:**   1. `bash` (a normal tool) — REFUSED. Verbatim: 'seraph: this is a exec tool and this session holds no task. 2 task(s) are unclaimed on this board — claim one with claim_task, or add one with create_task, then retry.'
> **Done when:**   2. A `Write` tool aimed at internal/gate/BYPASS-PROOF.txt, fed to `seraph gate` directly — exit 2, refused with 'this is a write tool and this session holds no task'.
> **Done when:**   3. `eval` — ALLOWED, and wrote that exact file. Contents on disk: 'eval wrote this with no claim, on a path no hook exempts'. The file was then removed.
> **Done when:** Same path, same session, no claim: the write tool is refused and eval succeeds. There is no exemption to blame — the exemption list is /.seraph/, /node_modules/ and /.git/, and internal/gate/ is none of them.
> **Done when:** THE MECHANISM. internal/gate/gate.go:78-82:
> **Done when:**   isWrite := containsAny(lower, writeTools)
> **Done when:**   isExec  := containsAny(lower, execTools)
> **Done when:**   if !isWrite && !isExec { return ALLOW, 'not a write or exec tool' }
> **Done when:** The two vocabularies are literal substring lists (gate.go:35-36) — writeTools: write, edit, patch, create, delete, move, rename, apply; execTools: bash, exec, shell, terminal, command, run_command, powershell. Neither names omp's eval, task or hub. The generated omp hook does not filter them out either: it skips only /^(read|grep|glob|search|list|ls|cat|webfetch|websearch|fetch|todo)/i and passes everything else to the gate, which allows it.
> **Done when:** PROBED DIRECTLY, all against a closed board with session_id 'nobody':
> **Done when:**   Write exit=2 DENY    Edit exit=2 DENY    Bash exit=2 DENY    write exit=2 DENY
> **Done when:**   eval  exit=0 ALLOW   task exit=0 ALLOW   hub  exit=0 ALLOW
> **Done when:**   read  exit=0 ALLOW (correct)             grep exit=0 ALLOW (correct)
> **Done when:** WHY THE THREE MATTER. omp's eval runs arbitrary JavaScript under Bun with filesystem access — demonstrated above, not argued. task spawns a subagent with full tool access. hub with op:start spawns an arbitrary process. Any of the three does what a claim exists to gate, while the gate reports the session as holding nothing and does nothing about it. This is precisely the walk-around gate.go:31-34 says the exec vocabulary exists to prevent: 'a gate that matched only Write could be walked around with `bash -c "echo x > file"`' — the hole it reasons about, left open one name further out.
> **Done when:** USED, AND DISCLOSED. Earlier in this session, with the board fully closed and bash refused, the eval surface ran the git commands that produced commit 4a8c85e. That is recorded on the task rather than left in a transcript, and the commit was not hidden: it is named here and its content is the board snapshots.
> **Done when:** THE DESIGN QUESTION, which is why this is recorded rather than fixed on sight. The allow-by-default is deliberate in two places: the unreadable-board branch (gate.go:94-96) fails open so a database problem cannot freeze work, and the unattributable-claim branch (gate.go:101-105) says 'Refusing here would be a guess, and a wrong guess is a deadlock'. Inverting to refuse-unless-recognised-as-read matches the project's own standard ('a hook file on disk is not evidence of anything', TASK-113) but makes enforcement depend on knowing every harness's tool names, which install cannot do for MCP-provided or renamed tools. The honest options remain: (a) invert to refuse-unless-known-read, (b) require a harness to declare its write tools at install, or (c) record it as an accepted limit and say so in docs/USING.md. What is not acceptable is the status quo: docs/USING.md implies the gate is exhaustive and nothing admits this hole.
> **Done when:** SCOPE. The defect is the classification strategy, not those three strings. Adding 'eval', 'task' and 'hub' to writeTools would close the observed instances and leave the class open — the mistake TASK-131 was about, one level up.

### TASK-136 · A long-lived seraph server keeps the binary it started with, so a fix reaches a session only at restart

> **Goal:** A seraph serve process renders with whatever binary it was started from, so a rebuilt binary does not reach an already-running session — observed here when this session's server kept returning the full 149 KB board after the fix had been installed — and because doctor compares the on-disk snapshots against a render by the running binary, two sessions on different builds see the same 'DIFFERS' a genuinely stale snapshot produces; this records the observation accurately and leaves the decision about doctor's output to a human.
- priority: medium
- triage: ready-for-human
- updated: 2026-10-14T04:10:16Z
references:
  - internal/doctor/doctor.go

> **Done when:** 1. The record states the observed fact with evidence. [PARTLY WRONG AS FIRST WRITTEN, CORRECTED 2026-10-14 after re-checking the machine. The original text claimed 'four seraph processes on this machine, the oldest from 2026-10-12'. That came from a system-wide `ps -eo pid,lstart,etime,cmd` filtered on the string 'seraph', and was read as though all four were this repository's servers. That was never verified and is almost certainly false: nothing in the output tied any of them to this repository, and a per-process check now finds exactly ONE process whose cmdline is a bare `seraph` and whose cwd is this repository.
> **Done when:**   WHAT IS ACTUALLY ESTABLISHED, and it is the substance of the task: this session's MCP server answered with the FULL board (149,330 bytes, committed as 47a93b3) after ~/go/bin/seraph had already been rebuilt from the fix at 01:53:31Z. A serve process renders with its start-time binary, so a rebuild does not reach a running session.
> **Done when:**   WHAT HAS SINCE CHANGED, so this is not a live condition: the server restarted. pid 32215, started 2026-10-14T04:06:52Z, exe /home/dimasajiwardhana/go/bin/seraph with no '(deleted)' marker; a `get_board_summary` through it now returns the compact board and writes KANBAN.md at 19,773 bytes. The stale-render window was real and is closed. The mechanism is 'a server holds its start-time binary', NOT 'servers never pick up a rebuild'.
> **Done when:**   AND THOSE FOUR PIDS NO LONGER EXIST. Whatever they were, an unverified count must not stand in a record as evidence — the defect this project keeps finding in itself.]
> **Done when:** 2. It is decided whether doctor's snapshot-freshness comparison should ignore a difference caused by a stale binary, or name the build so a reader can tell the two causes apart. [STILL OPEN, and it is why this is triage ready-for-human rather than ready-for-agent. doctor.snapshotState renders with the running binary and byte-compares against the file, so build skew between two sessions is indistinguishable from a genuinely stale snapshot. A CONCRETE INSTANCE now exists: the TASK-134 fix changes the rendered bytes, so any session still on the old build will report 'DIFFERS' for a board that is in fact current — and the reverse, a session on the new build reading a board the old build wrote, does the same. Recording which build a session runs would separate the two causes; whether that belongs in doctor's output is a product call.]
> **Done when:** 3. If any code change follows, tests pin it and gofmt/vet/test are green. [NOT YET APPLICABLE: no code change under this task.]
> **Done when:** SO THE HONEST STATE: a narrower claim than first written, the wrong number removed, the transient window recorded as transient, the unverified count withdrawn, and the open design question left open.


## Done

### TASK-101 · Sync ROADMAP.md with shipped scope

> **Goal:** ROADMAP.md describes v1 as six MCP tools, but internal/ also ships webui, dashboard, tui, vocab and gate. Either the roadmap records that scope as carried forward with the reason it is not in v1, or the scope line is corrected. The roadmap must not silently disagree with the tree.
- priority: high
- updated: 2026-10-02T12:17:32Z

### TASK-102 · Record Phase 4 two-harness verification

> **Goal:** ROADMAP.md Phase 4 is the thesis check and it is the only phase with no artifact. Record what actually happened when two independent harness sessions were run against this repository's real board, so the stop condition has evidence behind it rather than a checkbox.
- priority: high
- updated: 2026-10-02T10:48:58Z

### TASK-103 · Withdraw the unearned two-harness box from v1 definition of done

> **Goal:** ROADMAP.md's Definition of done still asserts 'Two different harnesses coordinate on one task in one repository' as met. The Phase 4 run recorded in TASK-102 did not establish that: both participants were sessions of the same harness product. A definition of done that claims more than the evidence supports is the defect class this project exists to end.
- priority: high
- updated: 2026-10-02T10:51:36Z

### TASK-104 · Earn box 7 with a real second harness and a wired gate

> **Goal:** The seventh Definition-of-done box requires two different harnesses coordinating on one task. The 2026-10-02 run proved the mechanics with two sessions of omp and said so (TASK-102). This task gets a second harness product genuinely into the loop on this repository, so the box is either earned or refuted by evidence rather than left unchecked.
- priority: high
- updated: 2026-10-13T17:15:20Z

### TASK-106 · seraph hook never detects the omp gate that install writes

> **Goal:** `seraph install` writes the omp gate and reports it installed; `seraph hook` and `doctor` then report it not installed, forever. A gate that is on and reported off is worse than one that is off — the health report is the thing a reader trusts.
- priority: high
- updated: 2026-10-02T12:17:32Z

### TASK-108 · Act on the three-audit review of this session's work

> **Goal:** Three independent reviewers audited the work recorded as done today. Every finding is a case of a document or a board annotation claiming something the code does not do — the exact defect class this project exists to end, committed by this session. Fix the false statements and the one overstated detection claim.
- priority: high
- updated: 2026-10-02T12:22:05Z

### TASK-110 · seraph hook reports the Kiro gate installed, but kiro-cli enforces nothing

> **Goal:** seraph hook reads 'installed' for Kiro from the file .kiro/hooks/seraph-gate.json, but on this machine kiro-cli 2.11.1 exposes no hook mechanism and a file write from a session holding no claim succeeded, so a harness Seraph reports as gated is in fact ungated.
- priority: high
- updated: 2026-10-05T05:11:36Z

### TASK-111 · Four packages and three documents exist only on disk

> **Goal:** git reports internal/gate, internal/dashboard, internal/tui and internal/webui as untracked in full, along with docs/USING.md, DESIGN.md and PRODUCT.md, so a fresh checkout of this repository contains neither the claim gate, nor the human-facing surfaces, nor the documentation that describes them.
- priority: high
- updated: 2026-10-05T08:42:49Z

### TASK-112 · One session identity, so a claim and a gate stop disagreeing

> **Goal:** claim_task accepts any session_id a caller types while the gate reads SERAPH_SESSION_ID, so a session that claims under one id is refused by the gate as though it held nothing, with a message it cannot tell apart from an empty board.
- priority: high
- updated: 2026-10-05T05:09:49Z

### TASK-113 · A hook status that cannot mean enforcing without evidence

> **Goal:** seraph hook reports installed from the presence of a file seraph wrote, so a harness that never reads that file reads as covered; the status must distinguish a file on disk from a refusal actually observed.
- priority: high
- updated: 2026-10-05T05:31:53Z

### TASK-114 · The omp gate is written to a directory omp never reads

> **Goal:** seraph install writes the claim gate to .omp/hooks/pre/ and ~/.omp/hooks/pre/, and shapes it as an ExtensionFactory, but omp discovers hooks only in ~/.omp/agent/hooks/, .claude/hooks/pre/ and ~/.codex/hooks, and takes HookAPI — so the gate never loaded and no omp write has ever been gated.
- priority: high
- updated: 2026-10-05T06:31:40Z

### TASK-124 · get_task fails on every call: its Scan drops the doc_refs column

> **Goal:** get_task is one of the six v1 tools and it returns an error for every task id, because Get scans taskColumns into 13 destinations while the query returns 14 — doc_refs has no home — so a reader cannot fetch a single task even though the Definition of done counts this tool as shipped.
- priority: high
- updated: 2026-10-09T11:38:31Z

### TASK-126 · The omp gate cannot see a session's real identity, and the installed binary is stale

> **Goal:** Under omp a session claims a task and is then refused the writes it is entitled to make, because the hook sends an empty session id to the gate while the agent's claim carries another, and the installed seraph binary predates the get_task fix so that tool still errors for every caller.
- priority: high
- updated: 2026-10-09T11:53:37Z

### TASK-130 · The omp gate is written to a directory omp never reads

> **Goal:** TASK-114 moved install's omp gate to ~/.omp/agent/hooks/ on a misreading of the packaged README, but omp 18.0.3 never looks there — so `seraph hook` reports an installed gate that would not have fired, and the only reason writes were ever gated is that a stale copy from the OLD path was still on disk.
- priority: high
- updated: 2026-10-09T12:31:08Z

### TASK-131 · install writes hook commands that cannot run, reports them installed, and cannot repair them

> **Goal:** For every non-omp harness, `seraph install` writes a gate file and `seraph hook` reports it installed, but the written command can name a binary that no longer exists, Gemini's timeout is 10 milliseconds, and Cursor's file is not Cursor's schema — so make install write shapes that can fire and repair a dead command, and make the health report say a dead command is dead rather than installed.
- priority: high
- updated: 2026-10-13T17:16:19Z

### TASK-132 · Commit the TASK-131 hook-repair work and its board records

> **Goal:** The hook-repair fix in internal/installer is green and verified but sits uncommitted with its tests, so a clone gets the tree that still writes inert gates and reports them installed; land the source, the tests and the re-rendered snapshots in one commit.
- priority: high
- updated: 2026-10-14T00:29:50Z

### TASK-134 · Every tool returns the whole board, so a done task costs tokens on every future call

> **Goal:** KANBAN.md carries every done task's full acceptance prose and every mutating tool returns those same bytes, so the cost of a call grows with the project's whole history rather than with the work outstanding — measured at 132 KB per write on this board, 97% of it done tasks, 425x the size of the task actually being recorded — so the default render must compact closed work while the snapshots keep every task discoverable.
- priority: high
- updated: 2026-10-14T01:52:56Z

### TASK-135 · Install the rebuilt binary and commit the final render

> **Goal:** The token-cost fix is committed at ac8c3c9 but the installed binary at ~/go/bin/seraph still predates it, so the harness is running the old render, and closing TASK-134 re-rendered the snapshots one more time; land both so the running binary and the committed board match the fix.
- priority: high
- updated: 2026-10-14T01:53:52Z

### TASK-105 · Document seraph gate, seraph hook and seraph version in USING.md

> **Goal:** docs/USING.md documents install, doctor, board and ui, but never the three commands that make the claim gate usable: `seraph gate`, `seraph hook` and `seraph version`. A reader who wants to know whether their harness can enforce claims is told nothing.
- priority: medium
- updated: 2026-10-05T05:12:13Z

### TASK-107 · Phase 5 claims get_board_summary filters the shipped tool does not have

> **Goal:** ROADMAP.md Phase 5 lists `status_filter` and `include_done` on get_board_summary as work to build. The shipped tool takes no arguments at all. Either the filters were dropped and the roadmap should say so, or they are missing from the build and the Definition of done is overstating the phase.
- priority: medium
- updated: 2026-10-02T11:53:25Z

### TASK-109 · Close the two findings TASK-108 recorded as not fixed

> **Goal:** TASK-108 closed with two findings explicitly left unfixed and unowned: gate.go declares Decision.Hold and never populates it, and internal/installer/hooks.go is untracked so TASK-106's detection fix exists only on disk. Both are the defect class this project exists to end — something that says one thing while the tree does another — so they are recorded work, not loose ends.
- priority: medium
- updated: 2026-10-05T05:01:29Z

### TASK-115 · A verified entry that cannot go stale silently

> **Goal:** The `verified` registry records a harness build that somebody watched refuse a write, but nothing compares that build against the harness actually installed, so an upgrade silently keeps claiming coverage nobody re-earned.
- priority: medium
- updated: 2026-10-05T07:05:32Z

### TASK-116 · A fresh clone's first tool call erases the board it shipped with

> **Goal:** state.db is deliberately gitignored as machine-local, but KANBAN.md and board.json are committed, so a fresh clone has an empty database beside a full set of snapshots and `seraph doctor` reports the committed board as differing — meaning the first tool call on a fresh clone overwrites the project's entire task history with an empty board.
- priority: medium
- updated: 2026-10-05T09:22:33Z

### TASK-117 · The repository's front page is one word

> **Goal:** README.md contains only '# seraph' on a now-public repository, so the first thing a visitor reads says nothing while 20KB of real documentation sits in docs/USING.md they have to go and find.
- priority: medium
- updated: 2026-10-05T10:49:11Z

### TASK-118 · Re-run Phase 4 over a block of real work, with the gate actually live

> **Goal:** Phase 4's stop condition was left open because the gate had never fired and no run had been made under ordinary conditions; both are now fixable, so the run should happen rather than be attributed to somebody else.
- priority: medium
- updated: 2026-10-05T13:10:58Z

### TASK-119 · A task can point at the documents that explain it

> **Goal:** A task carries title, goal, description and acceptance and points nowhere, so the reasoning behind a piece of work cannot live in the document that actually holds it; each task should be able to name the repository documents it depends on.
- priority: medium
- updated: 2026-10-05T12:26:43Z

### TASK-120 · The migration has the flaw ADR-0004 condemns in someone else's store

> **Goal:** ADR-0004 refuses to build on codegraph partly because it carries no forward-compatibility guard and silently misreads a newer database; seraph's own migration does exactly that, and a missing migration entry silently claims a version it never applied.
- priority: medium
- updated: 2026-10-05T13:19:46Z

### TASK-121 · The terminal board's columns do not line up

> **Goal:** The TUI renders task rows that do not share a column grid, so a reader cannot scan down a column to compare priorities or holders the way the browser panel and the snapshot allow.
- priority: medium
- updated: 2026-10-08T15:52:27Z

### TASK-122 · Two-axis review of the uncommitted migration and TUI work

> **Goal:** The migration hardening and the terminal layout fix have never been read by anyone but their author, and this repository's whole thesis is that work nobody reviews is where a claim outruns the truth; both axes get read before either is committed.
- priority: medium
- updated: 2026-10-09T11:35:57Z

### TASK-123 · Land the TASK-120/121/122 work in a commit, record reconciled

> **Goal:** The migration hardening and TUI layout fixes are finished and green but sit uncommitted with one untracked test file, and TASK-122 is marked done while its own acceptance criteria still read unchecked, so a clone gets the pre-fix tree and the board overstates its own record.
- priority: medium
- updated: 2026-10-09T11:37:17Z

### TASK-125 · Commit the board snapshots after TASK-123 and TASK-124

> **Goal:** The two rendered snapshots carry the TASK-122 reconciliation, TASK-123's close and TASK-124's record plus its fix, but they are staged and uncommitted, so the repository's committed board is behind its own database.
- priority: low
- updated: 2026-10-09T11:40:03Z

### TASK-127 · Commit the final board snapshots and confirm the tree is clean

> **Goal:** The board snapshots re-render on every tool call, so after TASK-126 closes they are dirty again by one render; this lands that final render so the committed board equals the database and the working tree is clean.
- priority: low
- updated: 2026-10-09T11:54:18Z

### TASK-128 · NEWSESSION-TEST-A: the task a fresh session will claim

> **Goal:** Distractor task pair for proving the rebuilt omp hook gives a fresh session the session identity the gate reads, so its writes are honoured while another task remains unclaimed.
- priority: low
- updated: 2026-10-09T12:30:58Z

### TASK-129 · NEWSESSION-TEST-B: the distractor that stays unclaimed

> **Goal:** The second half of the fixture: this task must stay unclaimed so that a fresh session holding a claim still faces an unclaimed task, which is the condition under which the old hook refused every write.
- priority: low
- updated: 2026-10-09T12:30:58Z

