---
id: "15-harden-shared-environment-recovery"
title: "Harden shared-environment recovery"
status: completed
wave: 7
depends_on: ["14-verify-status-customization"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 15: Harden Shared-Environment Recovery

## Acceptance

- Task LSP environment lookup ignores deleted historical environment references while rejecting
  multiple surviving physical environments.
- Reset cannot cross a borrower LSP admission, destroy a warm borrower's environment, or invalidate
  an active workspace-group environment pointer.
- Docker borrowers project repository roots from the physical task-host order, not their own task
  ordering, and incomplete legacy mappings fail closed.
- Task deletion can remove internal runtime credentials after the environment row is cascade
  deleted; durable cleanup snapshots retain the secret references required for retries.
- Task environment endpoints authorize the task owner before runtime access and map denial to a
  non-leaking not-found response.
- Monaco's toolbar controls the task derived from its editor session, never the globally active
  task. A missing session mapping exposes no control.
- Every WebSocket subscription establishment refreshes stable task LSP state. Revision ordering
  rejects a delayed REST response after newer live evidence.
- Capacity accepts a new backend epoch through an authoritative snapshot even after wall-clock
  rollback, then rejects delayed live evidence from the prior epoch.

## Verification

Completed 2026-08-12.

- Independent review: GPT-5.6 Sol reviewed the immutable pre-fix head in a separate Kandev task and
  workspace, producing eight actionable findings covered by this task.
- Backend focused packages: `go test ./internal/task/service ./internal/task/handlers
./internal/task/repository/sqlite ./internal/orchestrator/executor
./internal/agent/runtime/lifecycle ./internal/backendapp` passed.
- Race-focused lifecycle cases: `go test -race ./internal/task/service ./internal/lsp
./internal/agentctl/server/lsp -run 'Test(...)$' -count=1` passed, including physical-environment
  admission/reset ordering and concurrent task-language controls.
- Frontend regressions: five focused files passed 27 tests. Web typecheck and lint passed with zero
  diagnostics; `i18n:check` and `i18n:ratchet` passed.
- `make test` exercised the repository broadly but the local workspace filesystem made the
  orchestrator and task-service packages exceed Go's 10-minute package timeout while closing
  SQLite fixtures. No assertion failed; both affected packages passed in the focused command.
- Canonical exact-head CI and a second independent Sol review run after the remediation push; their
  results remain PR delivery evidence rather than changing this completed implementation record.

## Files

- `apps/backend/internal/task/service/**`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/executor/**`
- `apps/backend/internal/agent/runtime/lifecycle/**`
- `apps/backend/internal/task/handlers/**`
- `apps/web/hooks/domains/lsp/**`
- `apps/web/hooks/use-lsp.ts`
- `apps/web/components/editors/**`
- `apps/web/lib/state/slices/lsp/**`
