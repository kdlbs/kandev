---
id: "02-live-queue-capacity-settings"
title: "Add live install-wide queue capacity settings"
status: completed
wave: 2
depends_on: ["01-authoritative-queue-cancellation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-management.md"
---

# Task 02: Add Live Install-Wide Queue Capacity Settings

## Acceptance

- Effective capacity resolves as valid environment, persisted setting, then
  default `10`; malformed environment data is ignored with a warning.
- Non-positive environment values normalize to unlimited `0`; persisted
  negative values are rejected.
- GET returns configured and effective values, source, and lock state. PATCH is
  admin-only, rejects an environment lock, persists, and applies live.
- Queue status and every new-admission path observe one concurrency-safe live
  cap snapshot.
- Lowering capacity never removes rows. New admission blocks until below cap,
  while restore and durable retry of accepted work remain possible.
- Restart resolution applies the same value before the orchestrator accepts
  work.

## TDD Sequence

1. Add pure resolver and store tests for default, setting, environment,
   malformed data, unlimited normalization, and validation. Run RED.
2. Add service/handler tests for GET, admin PATCH, member rejection,
   environment lock, persistence failure, and live-target application. Run RED.
3. Add queue-service tests that change the cap concurrently, lower it below a
   current queue, and restore/retry accepted work. Run RED.
4. Implement `internal/system/queuesettings`, the atomic queue-service setter,
   startup resolver, and system route wiring.
5. Rerun focused tests GREEN, then run backend package tests covering startup
   composition and system routes.

## Verification

```bash
cd apps/backend
go test ./internal/system/queuesettings ./internal/system ./internal/orchestrator/messagequeue ./internal/backendapp
go test -race ./internal/system/queuesettings ./internal/orchestrator/messagequeue
```

## Files Likely Touched

- `apps/backend/internal/system/queuesettings/types.go`
- `apps/backend/internal/system/queuesettings/store.go`
- `apps/backend/internal/system/queuesettings/service.go`
- `apps/backend/internal/system/queuesettings/handler.go`
- matching `*_test.go` files
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/system/system_routes_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

Task 01 lands first so this task can update queue-service tests without
conflicting with cancellation contract changes.

## Parallelism

Sequential after Task 01. Startup resolution, HTTP service, and live queue
target form one consistency boundary.

## Output Contract

Report resolver precedence, auth/lock behavior, race-test result, live cap and
restore evidence, changed files, and any startup fallback. Update this task and
`plan.md` status in the same implementation conversation.

## Results

- RED: queue restore failed with `queue full` after the cap was lowered.
- RED: resolver ignored valid, unlimited, malformed, and locking environment
  values; the HTTP lock case returned `200` instead of `409`.
- RED: startup resolution ignored persisted settings and returned negative
  environment values unchanged.
- RED: System Message Queue routes returned `404` before registration.
- GREEN: `go test ./internal/system/queuesettings ./internal/system ./internal/orchestrator/messagequeue ./internal/backendapp -count=1`.
- GREEN: `go test -race ./internal/system/queuesettings ./internal/orchestrator/messagequeue -count=1`.
- Verified live lowering without pruning, all new-admission paths, unlimited
  mode, restore/retry bypass, environment precedence/lock, persistence-before-
  apply, and member/admin routing.
- Final spec audit added a RED/GREEN regression for malformed JSON and negative
  persisted values: GET now falls back to the default with a warning, and an
  admin PATCH can replace the invalid record without restarting.
- Final concurrency audit added a deterministic RED/GREEN regression for two
  overlapping admin saves: persistence and live application now share one
  serialized service boundary, so an older request cannot overwrite the live
  value after a newer request has persisted.
