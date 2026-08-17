## Fixed during review

* [apps/backend/internal/orchestrator/event_handlers_workflow.go:1974](apps/backend/internal/orchestrator/event_handlers_workflow.go:1974) — Derive a stable ID for legacy pending moves that predate `move_id`, preventing stale snapshots from replaying them (commit df61caf4c).
