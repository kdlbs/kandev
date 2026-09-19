---
id: "01-persist-descriptors"
title: "Persist Quick Terminal descriptors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 01: Persist Quick Terminal descriptors

## Acceptance

- A durable SQLite repository stores user/workspace/tab identity, non-reused sequence, session
  association, and bounded lifecycle fields with user/workspace uniqueness.
- Repository/service operations are idempotent where specified, authorize ownership through the
  supplied user/workspace boundary, and reconcile live versus stale host-shell sessions without
  starting a replacement.
- The host-shell manager exposes only the bounded lookup/key helper required by this service;
  existing agent-login uniqueness and public agent identity remain unchanged.

## Verification

```bash
(cd apps/backend && go test ./internal/quickterminal/... ./internal/agent/loginpty/...)
```

## Files likely touched

- `apps/backend/internal/quickterminal/models/tab.go` (new)
- `apps/backend/internal/quickterminal/repository/sqlite.go` (new)
- `apps/backend/internal/quickterminal/repository/sqlite_test.go` (new)
- `apps/backend/internal/quickterminal/service.go` (new)
- `apps/backend/internal/quickterminal/service_test.go` (new)
- `apps/backend/internal/quickterminal/handler_test.go` (new)
- `apps/backend/internal/agent/loginpty/manager.go`
- `apps/backend/internal/agent/loginpty/manager_test.go`

## Dependencies

None.

## Parallelism

Sequential. The repository and manager lookup form the backend contract consumed by every later
task.

## Inputs

- Spec sections: Data model, State machine, Permissions, Failure modes, and Persistence guarantees.
- ADR-2026-08-05-server-owned-quick-terminal-descriptors.
- Existing `apps/backend/internal/terminal/repository` schema and test patterns.
- Existing `loginpty.Manager.StartWithKey` internal-key separation and rolling-buffer lifecycle.

## Output contract

Report model/repository/service files, migration/ownership decisions, exact test result, and any
backend contract risk. Update this task and the parent plan only after the focused tests pass.

## Results

- Implemented durable user/workspace/tab descriptors, atomic non-reused sequences, lifecycle reconciliation, and manager-key ownership checks.
- Added host-shell client-ID idempotency while preserving the legacy singleton and stable `_host_shell` public identity; native Windows shell selection now covers PowerShell, `COMSPEC`, and `cmd.exe`.
- `(cd apps/backend && go test ./internal/quickterminal/... ./internal/agent/loginpty/... -count=1)` passed (21 test cases/subcases).
