---
id: "02-boot-recovery-and-error-writers"
title: "Boot recovery for error plugins + write/clear last_error"
status: done
wave: 2
depends_on: ["01-store-last-error"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 02: Boot recovery for error plugins + write/clear last_error

## Acceptance

1. `StartActivePlugins` attempts one spawn for managed plugins in `StatusActive` **or** `StatusError` that are not already running; success → `active` + cleared last_error; failure → `error` + non-empty last_error/last_error_at.
2. Spawn/handshake failure paths (`activate`, boot spawn, and unhealthy `handleStatusChange` at minimum) persist last_error; successful transition to active clears it.
3. Status `disabled` (sideloads) is never auto-spawned at boot. Enable still recovers error → active via `activate`.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/ -count=1 -run 'TestService(StartActivePlugins|Activate|Enable|HandleStatusChange|Install)'
```

Also run full package if partial is green:

```bash
cd apps/backend && go test ./internal/plugins/ -count=1
```

## Files likely touched

- `apps/backend/internal/plugins/service.go` (`StartActivePlugins`, `activate`, `handleStatusChange`, helper to set/clear error fields)
- `apps/backend/internal/plugins/service_test.go`
- Possibly `service_sync.go` (`markMissing`) if missing-install should set a reason
- Possibly `registry.go` if a dedicated SetLastError mutator is cleaner than Get-mutate-Save

## Dependencies

task-01 (Record fields must exist).

## Parallelism

sequential.

## Inputs

- Spec: State machine → Boot recovery, Last failure reason; Failure modes; Scenarios for boot resume, spawn failure, disabled sideload.
- Plan: Backend → Service lifecycle.
- Existing tests: `TestServiceStartActivePluginsSpawnsOnlyActiveManagedNotAlreadyRunning`, activate/handleStatusChange tests, `fakeRuntime` in service_test helpers.
- Enable already calls `activate` — keep that path; fix writers inside activate/boot.

## Implementation notes

- TDD order:
  1. Test: boot with persisted StatusError + healthy fake runtime → Running + Active + empty LastError (must fail before change).
  2. Test: boot with StatusError + failing Start → stays error + LastError set.
  3. Test: boot does not start StatusDisabled.
  4. Implement StartActivePlugins eligibility + shared activate/boot error recording.
  5. Ensure activate failure sets last_error; activate success clears any prior last_error.
  6. handleStatusChange(false): set a short reason (runtime callback is still `func(id, healthy bool)` — do not expand the callback signature unless trivial); handleStatusChange(true): clear last_error.
- One attempt per plugin per boot; log and continue on failure (no loop).
- Prefer small helper e.g. `setPluginError(id, reason string)` / `clearPluginError` that updates registry + store without fighting SetStatus FSM (error→error may need unchecked path like markMissing, or set last_error fields after a successful error transition only).

## Output contract

- Summary of lifecycle changes + new/updated test names
- Files changed
- Test commands + results
- Update task status `done` and plan checkbox
- Blockers / risks (e.g. error→error last_error refresh)
