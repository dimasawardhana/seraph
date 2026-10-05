<!-- SERAPH RULES START -->
## Working through the Seraph board

This project tracks its work with the `seraph` MCP server, which is the source of truth.
The files `.seraph/KANBAN.md` and `.seraph/board.json` are rendered from it — never edit
them directly; your edits are replaced on the next change.

**Write tools and shell commands are refused unless your session holds a live claim.** A
hook checks every `Edit` / `Write` / `apply_patch` / `Bash` call against the board before it
runs. Reading, searching and listing are always allowed.

That means the order is fixed:

1. **Before you touch anything, read the board.** `get_board_summary`. Do not duplicate
   work that is already tracked.
2. **If the work is not on the board, add it — before you start.** `create_task` needs a
   `title`, a `goal` (one sentence on what changes for whom when it is done), and
   `acceptance` (how anyone can tell it was finished correctly). The server refuses a task
   without all three, because a board that records only *what* happened cannot say whether it
   was the right work.
3. **Claim it.** `claim_task` with your `session_id`. You now hold it and your tools are
   allowed.
4. **Move it as you go.** `update_task` with `status`, `goal`, or `acceptance`. Every update
   you make refreshes your claim, so a long task never lapses under you.
5. **Finish with `update_task status: done`.** This releases the claim. A task can only be
   completed by the session holding it, which is what lets the board answer *who did this*.
6. **Give it back with `release_task`** if you are stopping without finishing.

A task already claimed by another session is not yours to take — surface that rather than
working around it. If you were blocked by the gate and the task genuinely exists, you were
asked to do work no one had recorded: that is a finding, not an obstacle.

Small, obvious work does not need a task. This applies to anything substantial: a bug fix, a
feature, a refactor, anything that changes behaviour someone will rely on.
<!-- SERAPH RULES END -->
