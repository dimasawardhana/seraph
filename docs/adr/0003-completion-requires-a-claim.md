# A task is finished only by the session holding its claim

Seraph refuses to move a task to `done` unless the session making the request holds an
unexpired claim on it, and refuses a claim on a task that is already claimed until that
claim expires.

The enforcement lives in the tool layer rather than in the installed Rules, because
instruction files are not a reliable channel. Harnesses disagree on which file they read,
some do not read any, and a harness that never read the Rules will not know the rule exists —
whereas a tool appears in the harness regardless of what the harness was told to read. The
Rules remain worth installing, but they cannot be the mechanism.

This also fixes what "the session" means. The protocol carries no ambient session identity:
a server learns the harness's name at initialize and nothing more. So the session is named
explicitly by the caller on every claim and every completion, and a completion is valid only
when that name matches the live claim.

**Callers get that name wrong, and the design assumes they will.** Two real harnesses racing
for one task were observed inventing the *same* session id — two processes of one harness,
each making up an identifier from the same environment. An earlier version let a re-claim
by the same session succeed, so the second process silently overwrote the first. The board
showed one holder and looked entirely correct, while both agents believed the task was
theirs. The failure was invisible *because* the board looked consistent, which is the
worst possible shape for a bug in a tool whose whole promise is that the board is not a
guess.

So a live claim is now refused whoever is asking, including its own holder, with
`CLAIM_ALREADY_HELD` saying so plainly rather than reporting a collision the caller does
not have. Nothing is lost: any mutation by the holding session already refreshes the
expiry, so a caller that legitimately still holds a task has no reason to re-claim it.

This prevents two harnesses from colliding by accident. It does not, and is not intended
to, defend against a caller that lies about its name. It only ensures that two callers
who agree on nothing else cannot both believe they hold the same task.

**Considered**: leaving transitions permissive and relying on the installed Rules. Rejected
because a Board recording unclaimed completions cannot answer who did the work and when,
which is the question the Board exists to answer.
