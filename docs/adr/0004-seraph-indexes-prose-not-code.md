# Seraph indexes prose; code intelligence is not duplicated

Code intelligence is already solved in this environment. `codegraph` is installed, local,
and already registered with nine harnesses. Seraph does not index source code and will not.

This is a permanent boundary, not a v1-versus-v2 sequencing decision. It was checked against
codegraph's actual storage rather than assumed, and the case is stronger than "somebody else
already built it":

- Its index is a SQLite database whose `open()` unconditionally applies WAL and other
  pragmas, so it **cannot be opened read-only** — not even through its own documented library
  API, whose `readOnly` option is declared in the type definitions and absent from the
  implementation.
- It carries **no forward-compatibility guard**. A reader shaped for the current schema
  version and pointed at a newer database silently misreads instead of failing.
- Its contents are **not purely factual**. Some edges are synthesised by heuristic rather
  than observed, and one table deliberately retains orphaned rows. Naive queries over them
  yield a call graph that is not the call graph.
- **Extraction semantics move on a version counter independent of the schema version**, so a
  structurally current database can still hold a semantically stale graph.
- The index is **bound to the operating system that wrote it**, under a single-writer lock
  held by a background daemon.

None of that is a criticism of codegraph. It is the ordinary cost of reading a private store
instead of an interface. The conclusion is only that the store is not a contract, and Seraph
should not build on it.

**The boundary costs nothing, because prose is genuinely uncovered.** codegraph's language
table contains no Markdown and no plain text; its full-text index covers symbol names,
signatures, and doc comments. The two tools therefore do not compete, and neither opens the
other's files. Their integration surface is empty.

A reader who later wants code intelligence in the same session should call codegraph — as a
process, over its CLI or MCP interface, treated as an opaque versioned service. That is the
only supported way in, and it is deliberately not a dependency of Seraph.

**Consequence to carry forward:** Seraph's binary ships FTS5 compiled in, and must not use it
for source code. Anyone extending a future walk must not add source extensions to it.
