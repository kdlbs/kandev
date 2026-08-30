---
id: "03-auto-merge-settings"
title: "Persist automatic merge setting"
status: completed
wave: 3
depends_on: ["02-auto-merge-admission"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 03: Persist automatic merge setting

## Acceptance

1. Message Queue settings GET/PATCH include `auto_merge_enabled` in configured
   and effective values; default and legacy-missing values are true, explicit
   false persists, and partial patches preserve capacity and manual merge.
2. Persistence completes before live apply, an environment capacity lock does
   not block an auto-only update or lose its effective live capacity, and
   backend startup applies the resolved values before new admissions.
3. Store, resolver, service, HTTP route, and backend wiring tests cover fresh,
   legacy, restart, invalid-record fallback, partial update, explicit false,
   live apply, and environment-lock cases.

## Verification

```bash
cd apps/backend && go test -count=1 ./internal/system/queuesettings ./internal/system ./internal/backendapp
cd apps/backend && go test -race -count=1 ./internal/system/queuesettings ./internal/orchestrator/messagequeue
```

## Files likely touched

- `apps/backend/internal/system/queuesettings/types.go`
- `apps/backend/internal/system/queuesettings/store.go`
- `apps/backend/internal/system/queuesettings/resolver.go`
- `apps/backend/internal/system/queuesettings/service.go`
- `apps/backend/internal/system/queuesettings/queuesettings_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/message_queue_settings_test.go`
- `apps/backend/internal/system/system_routes_test.go`

## Dependencies

Task 02.

## Parallelism

Sequential.

## Inputs

- Spec: `Data model`, `API surface`, `Permissions`, `Persistence guarantees`,
  and settings scenarios.
- Plan: `Backend > Persistent settings and startup wiring`.
- Existing patterns: pointer-backed `merge_enabled` legacy decoding,
  `SettingsPatch.Apply`, persisted-before-live-apply service flow, and
  `resolveQueueMergeEnabled` startup wiring.

## Risks

- Plain bool decoding would silently turn legacy installs off. The stored shape
  needs a pointer for the new field.
- Keep `merge_enabled` and `auto_merge_enabled` names and live setters distinct.
- The capacity environment source/lock metadata remains about capacity only.
- Live apply must use resolved effective capacity after any merge-only patch;
  copying the persisted capacity would override the active environment value.

## Output contract

Report summary, files changed, exact commands/outcomes, blockers, risks, and
update this task plus `plan.md` status in the same conversation.

## Results

- Added `auto_merge_enabled` to configured/effective responses and partial
  patches, with pointer-backed legacy decoding that defaults omitted values on.
- Persisted normalized three-field JSON and applied all live values only after
  a successful save.
- Fixed merge-only live apply under a capacity environment override to reapply
  the resolved environment capacity instead of the persisted value.
- Wired startup to resolve once and apply capacity, manual merge, and automatic
  merge before queue admissions; added fresh/legacy/false/restart resolver,
  HTTP, persistence, and live-target tests.
- Verification passed:
  - `cd apps/backend && go test -count=1 ./internal/system/queuesettings ./internal/system ./internal/backendapp`
  - `cd apps/backend && go test -race -count=1 ./internal/system/queuesettings ./internal/orchestrator/messagequeue`
- Blockers: none.
