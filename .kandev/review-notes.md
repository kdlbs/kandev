## Fixed during review
- apps/backend/internal/clarification/store.go:340 — closed a retry waiter that could race with cancellation and otherwise wait indefinitely after detached delivery won (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/mcp/handlers/handlers.go:3978 — allowed a finalized detached rejection to reconcile immediately instead of being masked by the delivery-miss receipt (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:116 -- auto-fallback decisions retained a configured fallback model in provider-default warning metadata; normalized the policy so auto-fallback ignores the fallback model consistently with the runtime contract (commit 649b4fa22)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:140 -- auto-fallback profiles failed when ACP model selection was unsupported; restored provider-default continuation with selection_unsupported warning metadata (commit fdfbffc9a)
- Session cleanup now broadcasts an archive state update with `archived_at` and retains the archived row in the client store. A terminal removal event is reserved for actual hard deletion, so open conversation subscribers keep retained history.
- Primary promotion now rejects archived sessions within the promotion transaction, preserving an active primary when a stale client tries to select history.

## QA fixes worth reviewer attention
- Mobile history capture waits for the sheet to settle, and the deletion confirmation has a rendered capture. A kanban-preview E2E journey verifies hidden and revealed terminal helpers.
- The task-session summary and full DTO now expose `archived_at`, including the bounded SQLite summary projection. An end-to-end journey archives a mounted helper conversation, verifies the archive state in the API, then reopens its retained transcript.
- The service fallback that builds summaries from full session rows also preserves `archived_at`; a fallback-path regression test covers repositories without the narrow summary reader.

## Follow-up tasks created
- None.

## Action required by author
- None for the local correction; normal PR, CI, and merge gates remain.

## Assumptions made during QA
- Repository E2E fixtures are disposable scenario data for UI verification. The reusable task instance uses an isolated SQLite database with authentication enabled.

## Pre-existing issues found
- None affecting this candidate.

## UI screenshots for PR/MR
- Final desktop, kanban-preview, and mobile captures are retained under `apps/web/.pr-assets/`; independent QA must inspect their actual pixels for the final commit.
