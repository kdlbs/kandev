## Fixed during review
- apps/backend/internal/clarification/store.go:340 — closed a retry waiter that could race with cancellation and otherwise wait indefinitely after detached delivery won (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/mcp/handlers/handlers.go:3978 — allowed a finalized detached rejection to reconcile immediately instead of being masked by the delivery-miss receipt (commit 3e5fd27b551a6f674604d68d23ff4ef1aff62c97)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:116 -- auto-fallback decisions retained a configured fallback model in provider-default warning metadata; normalized the policy so auto-fallback ignores the fallback model consistently with the runtime contract (commit 649b4fa22)
- apps/backend/internal/agent/runtime/lifecycle/start_model.go:140 -- auto-fallback profiles failed when ACP model selection was unsupported; restored provider-default continuation with selection_unsupported warning metadata (commit fdfbffc9a)
- apps/backend/internal/mcp/handlers/handoff_handlers.go -- bound related-task workspace-tree reads to the current caller principal, with task/session/surface and workspace checks; added raw-action and caller-workspace regressions (commit 35d6f0e097)

## Follow-up tasks created
- None for the coordinator relation-inspection continuation.

## Action required by author
- None for this Work-review-qa handoff. Native PostgreSQL and deployed-agent validation were not run in the task-local transient test environment; publication, CI, and later end-user runtime gates remain with their workflow owners.

## QA fixes worth reviewer attention
- RV-01: An ordinary Office task could request an unrelated task's compact relation tree through the raw backend action. The handler now validates scope and caller identity against the per-dispatch principal; QA confirmed denied and allowed paths at commit 35d6f0e097.

## Assumptions made during QA
- The approved scope is a same-workspace, compact, read-only relation view for an attested Office coordinator. Manual task-visibility grants remain a separate capability.

## Pre-existing issues found
- None identified in the task-owned delta.

## UI screenshots for PR/MR
- Not applicable: no visible or interactive UI change.
