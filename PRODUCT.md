# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

Surfaces: a browser dashboard and a terminal application. Both are real surfaces with
equal standing, not a primary and a fallback.

## Stack

Existing codebase answers this; recorded here for the record only. Go, embedded SQLite,
`net/http` with server-rendered HTML, and a hand-rolled terminal renderer using
`golang.org/x/term` for raw mode and `lipgloss` for display-width measurement. No
frontend build step, no framework, no client bundle. Styling is plain CSS in one file.

## Users

One person: the developer running the harnesses. Not a team, not an operator, not a
customer. They are the author of the board and its main dispatcher, in the same
repository they are building in.

They use Seraph because they already run several AI coding harnesses — Claude Code,
Cursor, opencode, others — against the same codebase, and those harnesses each keep a
private task list that dies with the session.

## Product Purpose

A durable, harness-independent board of a project's work. Harnesses read from it and
write to it instead of keeping their own state, so what a project learns outlives the
session that learned it.

Success is a task that one harness started and a different harness finished, with the
board naming who did it and when.

## Positioning

The mechanism a neighbouring product could not truthfully copy: **an exclusive,
expiring claim, enforced by the server.** A task cannot be completed by anyone but the
session holding its claim, and the refusal names the holder. Harnesses already ship todo
lists; none of them can refuse you. That refusal is what makes the board trustworthy
enough to dispatch real work from — and it is the reason three prior attempts at a board
in this environment failed, each of which stored status in prose or unconstrained text
so nobody could trust it.

## Operating Context

Two screens, used side by side. The dashboard runs in a browser on a second monitor
while the terminal is occupied by agent output; the terminal board is the quick check
between runs. Light and dark environments both occur.

The primary move on either surface is **dispatch**: scan for work nobody holds, claim
it, and push it along. Contention matters — an agent holding a task is common, not
exceptional — so "who holds this, until when" is load-bearing information, not
metadata.

## Capabilities and Constraints

- Six tools over MCP on stdio: `get_board_summary`, `get_task`, `create_task`,
  `claim_task`, `update_task`, `release_task`.
- Three axes of state, kept deliberately distinct: **status** (position on the line,
  5 values), **triage** (who can pick it up, 5 values or none), **priority**
  (urgency, 4 values).
- The claim rule is enforced in the tool layer, not in instructions written to disk.
  Rules text is advisory; the server is the mechanism.
- The web dashboard binds loopback only and requires a per-run token on every request.
- Claims expire (30 min default) and are refreshed by any mutation the holder makes.
- No authentication, no multi-user, no sync, no remote access.
- Document search is deferred to a later version, by decision, not by omission.
- Open: whether harnesses should be instructed to create tasks for work they start.
  Reproduced unfixed — an agent handed ordinary work does the work and records nothing,
  because the Rules describe only what to do with a task already found.

## Brand Commitments

The name is Seraph. The vocabulary in `CONTEXT.md` is binding domain language — Board,
Task, Status, Triage, Priority, Claim, Harness, Session, Snapshot — and must not be
relabelled for interface convenience. Prose is "task", never "issue" or "card".

## Evidence on Hand

- The product is real and running; both surfaces are implemented and verified against a
  real repository.
- No user testimonials, customer names, benchmarks, or pricing exist, and none may be
  invented.
- No brand assets, logo, or committed identity system exist. Any mark used in the
  interface is ours to author and is not a claim.

## Product Principles

1. **The board is a dispatch queue, not a report.** It earns its place by being acted
   from, not read.
2. **Refuse honestly.** A claim that cannot be granted says who holds it and until
   when. Silence would be worse than the failure.
3. **One vocabulary across both surfaces.** Same states, same actions, same words. A
   task is a task in the browser and in the terminal.
4. **Density over decoration.** Many tasks legible at once, state readable without
   clicking. Expression may never obscure the task.
5. **Every action has a key.** A second monitor beside a terminal is a split-attention
   surface; reaching for the mouse is a cost paid on every iteration.

## Accessibility & Inclusion

Keyboard-first is a confirmed requirement, not an enhancement: every action must be
reachable and operable from the keyboard, in both the browser and the terminal.

Full WCAG AA contrast is required in both light and dark. The incumbent light theme
failed this at 3.4:1 on its success banner and is not a baseline worth keeping.
