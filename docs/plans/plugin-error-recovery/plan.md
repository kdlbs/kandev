---
spec: docs/specs/plugins/spec.md
created: 2026-07-30
status: done
---

# Implementation Plan: Plugin last_error + boot recovery

## Overview

Managed plugins that enter `status=error` currently look permanently broken: the host does not persist why they failed, and boot never retries them. This plan adds `last_error` / `last_error_at` on the installation record, one boot recovery spawn for managed `error` plugins, and UI that shows the reason and offers Enable to retry.

Root cause (confirmed in code):
- `store.Record` has `status` + `restart_count` only (`apps/backend/internal/plugins/store/store.go`)
- `StartActivePlugins` only spawns `StatusActive` managed not-running (`service.go`)
- `activate` / `handleStatusChange` set `StatusError` without a reason string
- UI `canEnable` is only `disabled|registered` — error rows have Disable, not Enable

Order: persistence fields first (so API/YAML carry them), then lifecycle writers + boot recovery, then frontend types/UI. Tests are TDD on each task.

---

## Backend

### Store / record
- Add to `store.Record` in `apps/backend/internal/plugins/store/store.go`:
  - `LastError string \`yaml:"last_error,omitempty" json:"last_error,omitempty"\``
  - `LastErrorAt *time.Time \`yaml:"last_error_at,omitempty" json:"last_error_at,omitempty"\``
- FSStore Save/Get/List already round-trip the whole Record — no schema migration (YAML files).
- Registry must copy the new fields on SetStatus / any field-update helpers used when clearing or setting errors. Inspect `registry.go` for `SetRestartCount`-style mutators; add `SetLastError` / clear-on-active if status-only SetStatus would drop sibling fields incorrectly (SetStatus today only changes Status — sibling fields stay on the in-memory record, so writers must mutate then Save).

### Service lifecycle
- Central helper (e.g. `recordError(id, reason string) error` or fold into paths that set StatusError): set status error, set LastError + LastErrorAt=now UTC, Save, notify deliverer as today.
- Call sites that must write last_error:
  - `activate` spawn failure
  - `StartActivePlugins` spawn failure (active boot)
  - `handleStatusChange(false)` unhealthy (use a stable short reason if runtime callback stays `func(id, healthy bool)` — e.g. `"plugin unhealthy"` / restart exhausted; optional stretch: extend OnStatusChange with reason — only if low churn)
  - config restart failure path already SetStatus(Error)
  - `markMissing` / missing-install sync path
- Clear last_error + last_error_at on successful transition to StatusActive: `activate` success, `handleStatusChange(true)`, boot recovery success.
- `StartActivePlugins`: after bootScan, spawn when status is `StatusActive` **or** `StatusError`, managed, not running. Prefer reusing `activate` (or a shared spawn-and-status helper) so success→active+clear and fail→error+last_error stay consistent. Log failures; continue other plugins. One attempt per plugin per boot — no host loop.
- Enable already calls `activate` for non-active — remains correct for error→active once activate writes/clears last_error.
- Disabled sideloads must still be skipped (status != active/error).

### API
- List/get already return `*store.Record` / DTO embedding — new fields appear automatically if JSON tags are on Record. Confirm `dto.go` / handlers do not strip unknown fields.
- No new endpoints.

---

## Frontend

- `apps/web/lib/types/plugins.ts` `PluginRecord`: add optional `last_error?: string | null`, `last_error_at?: string | null`.
- `plugin-row.tsx` / `plugin-detail.tsx`:
  - `canEnable` includes `status === "error"`.
  - When `status === "error"` and `last_error` present, show the text (muted/destructive, `data-testid` for tests) under the status row.
- Prefer small presentational tweak in row + detail; no new API client methods (enable already exists).
- Unit tests: plugin-row.test.tsx (and detail if covered) for Enable on error + last_error text.

---

## Tests

| What | File | How |
|---|---|---|
| Record YAML/JSON round-trips last_error fields | `store/fs_store_test.go` | Save/Get with LastError + LastErrorAt |
| Boot resumes managed error → active on healthy spawn | `service_test.go` | Extend StartActivePlugins fixture: persist StatusError, StartActivePlugins, assert Running + Active + empty last_error |
| Boot spawn failure writes last_error, stays error | `service_test.go` | fakeRuntime Start fails; assert StatusError + non-empty LastError |
| Boot does not spawn disabled | `service_test.go` | disabled managed/sideload not started |
| activate/Enable failure writes last_error | `service_test.go` | install or Enable with failing runtime |
| Successful active clears last_error | `service_test.go` | handleStatusChange true or Enable after error |
| Existing StartActivePlugins active path still works | `service_test.go` | keep/adjust TestServiceStartActivePluginsSpawnsOnlyActiveManagedNotAlreadyRunning |
| UI Enable on error + shows last_error | `plugin-row.test.tsx` | render error plugin with last_error |

Pre-PR commands (per task files):
- `cd apps/backend && go test ./internal/plugins/... ./internal/plugins/store/...`
- `cd apps && pnpm --filter @kandev/web test -- components/settings/plugins/plugin-row.test.tsx`

---

## E2E Tests

Skip dedicated E2E for this fix: behavior is unit-covered host lifecycle + settings list presentation; no new route/flow. Optional follow-up only if product wants Playwright on Settings → Plugins error row.

---

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [x] [task-01-store-last-error](task-01-store-last-error.md)

Wave 2 (depends on 01):
- [x] [task-02-boot-recovery-and-error-writers](task-02-boot-recovery-and-error-writers.md)

Wave 3 (depends on 01; parallel-safe with 02 after types land):
- [x] [task-03-ui-last-error-and-enable](task-03-ui-last-error-and-enable.md)
```

Default execution: sequential 01 → 02 → 03 in the primary session. Task 03 is parallel-safe with 02 only if `PluginRecord` types from 01 are already merged (frontend only needs the JSON field names).

---

## Open Questions

None blocking. Optional later: extend `OnStatusChange` with a reason string for richer health failures; v1 can use a fixed unhealthy message.
