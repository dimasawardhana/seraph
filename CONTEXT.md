# Seraph

Seraph is a durable, harness-independent record of a project's work and knowledge. Harnesses
read from it and write to it instead of keeping their own state, so that what a project
learns outlives the session that learned it.

## Language

### Work

**Task**:
A single unit of tracked work. Carries a title, and may carry a longer description.
_Avoid_: issue, ticket, card, item, bug

**References**:
The documents a task points at — repository-relative paths, each optionally carrying an
anchor. The reasoning behind a task belongs in the document that holds it; the task names it
rather than copying it. Seraph does not hold these documents: they stay in the repository,
where git can review them.
_Avoid_: attachments, links, resources

**Board**:
Every task belonging to one repository, considered as a whole.
_Avoid_: kanban, project, backlog, list

**Status**:
Where a task sits on the line from unstarted to finished — `backlog`, `todo`, `in_progress`,
`review`, `done`. Exactly one per task.
_Avoid_: state, stage, phase, column

**Triage**:
A judgment about who is able to pick a task up right now — `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, `wontfix`. At most one per task, and often none.
_Avoid_: status, priority, label, tag

**Priority**:
How urgent a task is, independent of where it sits on the line and of who can take it —
`low`, `medium`, `high`, `urgent`.
_Avoid_: severity, weight, rank

**Claim**:
An exclusive hold on a task, taken by one harness session and expiring on its own, so that
two harnesses cannot both believe a task is theirs. Only the session holding the claim may
finish the task.
_Avoid_: assign, lock, checkout, reserve, ownership

**Harness**:
An AI coding client that reads from and writes to the board. Each is a distinct consumer,
with its own configuration, its own rules, and its own sessions.
_Avoid_: agent, client, IDE, tool

**Session**:
One continuous run of a harness. Identity is per run, not per harness: the same harness
running twice holds two unrelated claims.
_Avoid_: conversation, thread, run, instance

### Knowledge

**Context**:
The durable, shared record of a project's work and knowledge, held by Seraph rather than by
any one harness. The thing Seraph exists to preserve.
_Avoid_: memory, notes, docs

**Index**:
The searchable projection of a project's documents, so a harness can find prior knowledge
without reading the tree blind. An Index holds documents; only a Context holds work.
_Avoid_: context, vector store, RAG

**Snapshot**:
The rendered form of a Board, for consumers that cannot reach Seraph directly. Derived
from the Board and never an input to it.
_Avoid_: export, mirror, artifact, output

### Governance

**Rules**:
The block of instructions Seraph places in a repository's agent instruction files, telling
harnesses to work through the Board rather than around it. Advisory, and read by some
harnesses rather than all.
_Avoid_: policy, instructions, constitution, prompt
