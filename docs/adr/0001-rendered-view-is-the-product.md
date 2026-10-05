# The rendered board, not the database, is what consumers read

Seraph's tools return the same rendered board that gets written to disk, rather than
querying the database and returning rows. This makes the on-disk board and the tool
response the same view at every moment, which is the only way "single source of truth" holds
when there are two kinds of reader. The database is therefore an implementation detail of
the renderer rather than a surface harnesses touch.

A consequence follows: a write does not return until the rendering it produced is in place.
Rendering is synchronous. It is a single selection plus a few kilobytes of text, so this
costs nothing measurable, and it means a caller can read the board immediately after
changing it without racing anything.

**Considered**: returning rows from live queries, with the files as a derived convenience.
Rejected because a harness and a human reading the same board at the same moment could
disagree about what is on it, and neither would be wrong.
