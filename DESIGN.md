# Design — Seraph

<!-- impeccable:design-schema 1 -->

Surfaces: the browser panel (`internal/webui`) and the terminal board (`internal/tui`).
Both are one product in two rooms and must read as one. This document is written from the
built world, not before it.

## Mode

**Operate.** The visitor is dispatching work and is usually looking at an agent's output
at the same time. Expression may never obscure the task, its state, or a familiar
affordance. Brand lives in precise details, not in atmosphere.

## Thesis

**A dispatch board is a track diagram.** A task is a block; a claim is that block showing
occupied, with its holder and the time it clears. The surface's job is to make occupancy
legible at a glance before anything is clicked.

This is chosen against two defaults: the Trello clone — rounded cards in coloured columns,
drag handles, a "+" button — and its opposite, a stark terminal-chrome list. Neither is
wrong on its own; both are what a category ships without deciding anything.

## The world

**Enamel-painted signal panel.** Cool neutral greys, never warm cream. Rules at hairline
weight. State plates carry status the way a signal box carries a route: small, uppercase,
wide-tracked, and never competing with the work beneath them.

**Colour is signal, and only signal.** Four aspects, and nothing else is tinted:

| Aspect | Meaning | Token |
| --- | --- | --- |
| ○ clear | nobody holds it; yours to take | `--clear` |
| ● held | someone else's, with the clearing time | `--held` |
| ● caution | theirs, and it clears within 5 minutes | `--caution` |
| ● mine | yours; ringed, because this is the one that is personal | `--mine` |

Amber is never decoration here. It means *act soon or lose it*, which is a fact about the
board and nothing about the design.

**Type.** One workhorse system sans for chrome and data; a condensed grotesque in the
railway-lettering lineage is the intent, expressed through tracking and case rather than
through a webfont the project does not ship. Tabular numerals throughout — every count,
every duration, every identifier is read in a column. Monospace is reserved for values
that *are* data: task ids, durations, counts. Never as costume.

**Elevation is declared once**, as a 1px border. Radius ≤2px on controls and nowhere
else. No shadows, no glow, no glass. Rows are ruled, not carded: a board of rounded boxes
is a gallery, and a gallery is the wrong shape for twenty rows of state.

## Composition

The browser panel is one ruled grid:

```
rail      SERAPH · repo path · task count · yours · live pip
compose   new task · priority · triage · add
filters   filter by title, id, or holder · match count · keys
panel     status plate, then one row per task
foot      what the server enforces · key reminders
```

A row is fixed-width and holds: **aspect dot · id · priority · title · triage · holder and
clearing time · actions.** The title takes the slack; everything else holds its column, so
the eye reads a column of ids down the left and a column of holders down the right.

Status groups are labelled and counted, and an empty group is omitted rather than shown
blank — a column that exists only to say nothing is noise.

The terminal board is the same diagram in ANSI. Same column order, same aspects, same
vocabulary, same keys. A narrow terminal drops the priority column, then the clearing
time; it never wraps, because a wrapped line in a fixed-height panel pushes the board off
screen.

## Interaction

**Keyboard-first, confirmed as a requirement.** Every action has a key, and the key path
and the pointer path fire the same form, so they cannot drift:

`j k` move · `g G` ends · `c` claim · `s` start · `r` review · `d` done · `u` release ·
`n` new · `/` filter · `↵` open · `esc` clear · `?` keys

Focus *is* the selection. Focusing a row by click, Tab, or assistive technology moves the
cursor with it, so a key never acts on a row the user has left.

**The refresh cannot interrupt you.** The panel re-renders only when a content hash
actually changes; an idle poll does nothing at all. When it does change, selection and any
open detail survive. Typing in a field, hovering a row, and reading a description all
outlive every poll — verified by holding a half-typed filter across four ticks.

The live pip reports state rather than animating: filled when a poll landed, hollow while
unchanged, amber when the server stops answering.

## Surfaces and refusals

Both surfaces enforce the same rule, in the tool layer, not in instructions written to
disk: **a task can only be completed by the session holding its claim.** A human who is not
the holder gets the same `CLAIM_REQUIRED_FOR_DONE` an agent gets. A success note is written
only when the action actually succeeded — an earlier build printed a green "Completed"
beside the refusal that prevented it, and a reader takes the green line for the answer.

The browser binds `127.0.0.1` only, and every request carries a token minted per run. A
local endpoint that accepts form posts is reachable by any page open in the same browser.

## Anti-patterns held to

- No `border-left` accent bars thicker than 1px. The aspect dot carries state.
- No gradient text, glass, or glow. Emphasis comes from weight and colour-as-meaning.
- No emoji or Unicode standing in for an icon set.
- No modal for anything. Detail expands in place.
- No spinner in content. The board is never "loading"; it is current or stale, and says which.
- No per-card walls of buttons when the row can carry the actions that apply.

## Verification

Every claim above was checked rather than assumed: contrast against WCAG AA in both
themes, keyboard behaviour driven through a real browser, overflow measured at 1440 / 390
/ 70 / 50 columns, and the refresh measured by holding a field mid-poll. The bundled
detector reports clean.

Not verified, and honest about it: the open product question of whether the Rules should
tell harnesses to *create* tasks for work they start. That is a product decision, not a
design one, and it is recorded in `PRODUCT.md` as unresolved.
