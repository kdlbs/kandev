---
id: "01-add-session-capability"
title: "Add and enforce the session capability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-executor-availability.md"
---

# Task 01: Add and Enforce the Session Capability

## Acceptance

- A shared backend resolver returns the specified embedded-VS-Code capability for every known
  runtime/host-OS combination and fails closed for unknown values.
- Every authorized task-session status response includes
  `capabilities.embedded_vscode` for that session's executor.
- Opening `internal_vscode` directly or through the saved default returns
  `ErrEditorUnavailable` for an unsupported session while supported sessions and other editor kinds
  retain their existing behavior.

## Verification

Use TDD: add the executor matrix, status contract, and editor-service rejection tests; observe the
focused failures; implement the shared resolver and both consumers; then rerun from
`apps/backend/`:

```bash
go test ./internal/editors/capabilities ./internal/orchestrator ./internal/editors/service ./internal/editors/handlers
```

## Files likely touched

- `apps/backend/internal/editors/capabilities/capabilities.go` (new)
- `apps/backend/internal/editors/capabilities/capabilities_test.go` (new)
- `apps/backend/internal/orchestrator/dto/dto.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/editors/service/service.go`
- `apps/backend/internal/editors/service/service_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the backend contract and shared enforcement consumed by Task 02.

## Inputs

- Spec sections: **What**, **API surface**, **Failure modes**, and scenarios 1–7.
- ADR:
  `docs/decisions/2026-07-30-embedded-editor-executor-capabilities.md`.
- Existing patterns:
  - executor constants in `apps/backend/internal/task/models/models.go`
  - `TaskSessionStatusResponse` in `apps/backend/internal/orchestrator/dto/dto.go`
  - `populateExecutorStatusInfo` in `apps/backend/internal/orchestrator/task_operations.go`
  - `ErrEditorUnavailable` in `apps/backend/internal/editors/service/service.go`

## Risks

- Do not let unknown or test-only executor values inherit the default standalone runtime.
- Use actual session executor data for enforcement; do not trust a client-supplied executor type.
- Keep the editor service's repository interface minimal and update its test doubles without
  broadening unrelated dependencies.

## Output contract

Report the API shape and capability matrix implemented, files changed, red and green command
results, and any remaining risk. Update this task to `done`, its plan checkbox/status, and the spec
to `building`.

## Completion record

- Added `editors/capabilities.SupportsEmbeddedVscode`, which accepts only Linux/Darwin for Local
  and Worktree, accepts the supported container/remote executor types, and fails closed otherwise.
- `task.session.status` now always includes `capabilities.embedded_vscode`; missing executors
  return `false`. Direct internal-editor requests resolve the session executor and return
  `ErrEditorUnavailable` when unsupported, for saved-default, explicit, folder, and file paths.
- Red: the resolver/status/service tests initially failed to compile or returned a successful
  internal-editor URL before the capability contract and guard were added.
- Green: `go test ./internal/editors/capabilities ./internal/orchestrator ./internal/editors/service ./internal/editors/handlers` passed.
- Remaining risk: a true capability identifies a supported runtime only; code-server download and
  startup retain their existing runtime error handling.
