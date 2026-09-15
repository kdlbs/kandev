## Fixed during review
- apps/backend/internal/orchestrator/messagequeue/pending_move_exact_cancel.go:134 — Redacted a same-workspace pending row when the submitted session resolves to a task outside the authorized task-tree target. (commit 3f221d84f1d1831dc61ef348ab3655cdcdd8eab2)
- apps/backend/internal/mcp/handlers/pending_move_read.go:30 — Preserved identifier presence and canonicality in malformed census-request audit evidence. (commit 3f221d84f1d1831dc61ef348ab3655cdcdd8eab2)
