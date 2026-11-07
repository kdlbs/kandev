---
status: draft
created: 2026-11-07
owner: kandev
---

# Recorded task wait reasons

Task lists and board views distinguish a recorded WIP queue, session-capacity
queue, and deadline-backed provider quota wait. Explicit permission or
clarification and observed live activity keep their existing priority.
`WAITING_FOR_INPUT` alone is not proof that a question awaits an answer.

The quota projection reads the current session's logical profile and route
generation; historical routes and terminal sessions cannot supply a current
wait. A failed quota read does not establish an empty observation or block
updates from other task-status sources. Missing summaries are rebuilt with the
current wait; changes to existing summaries use their revisioned update path.
If another writer wins, the repair reads both the latest summary and route
before retrying.
