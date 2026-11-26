---
id: "10-release-toggle"
title: "Add the usage history release toggle"
status: done
wave: 4
depends_on:
  - "08-retire-legacy-write-path"
  - "09-shims-and-snapshots"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-007.8
  - AC-COSTS-USAGE-HISTORY-007.9
  - AC-COSTS-USAGE-HISTORY-007.10
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 10: Add the usage history release toggle

## Summary

Add `features.usageHistory` (`KANDEV_FEATURES_USAGE_HISTORY`), restart required, so retained usage can merge off and be disabled later. Follow `/runtime-feature-flags`.

## In scope

- `profiles.yaml` (`prod: "false"`, `dev: "false"`, documented `e2e: "true"`), `FeaturesConfig`, the registry, `runtimeflags/config.go`, and the frontend `types.ts` key.
- Backend gates: native admission in `usage_events.go`, `Preserve*Usage`, `UsageHistoryWorker` startup, the token usage route, `SetUsageSource`, and `UsageHistory()`.
- Frontend gates: `/stats/token-usage` route and navigation entry.
- Keep `Erase*Usage` and `Complete*UsageTx` running while off.

## Out of scope

- Promoting `prod` to `"true"`. This is a later user decision after real-install testing.

## Acceptance

- With the toggle off: no worker runs, no retained rows are written, the route and Host APIs are unavailable, and task deletion works.
- With the toggle off after use: retained rows stay, and workspace and organization erasure still remove them.
- With the toggle back on: admission resumes without duplicates.

## ASCII UI preview

The Stats navigation entry and `/stats/token-usage` route are absent when the toggle is off. There is no new visible design.

## Verification

```bash
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test ./internal/backendapp/ -run 'TestUsageHistoryToggle' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite/ -run 'TestUsageHistory' -count=1)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
(cd apps/web && pnpm e2e:run tests/layout/usage-history-toggle.spec.ts)
```

## Files likely touched

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/config.go`
- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/usage_history_toggle_test.go` (new)
- `apps/backend/internal/analytics/handlers/stats_handlers.go`
- `apps/backend/internal/task/repository/sqlite/usage_events.go`
- `apps/backend/internal/task/repository/sqlite/base.go`
- `apps/web/lib/state/slices/features/types.ts`
- `apps/web/src/spa-routes.tsx`
- `apps/web/e2e/tests/layout/usage-history-toggle.spec.ts` (new)
- `docs/public/usage-history.md`

## Dependencies

Tasks 08 and 09.

## Risks

- `usageHistoryLifecycle` currently controls both preservation and erasure through one nil check. Do not gate by leaving it nil, or erasure stops silently and breaks AC-COSTS-USAGE-HISTORY-001.6.
- Merging with `prod` off means no retention until promotion. Native usage accepted in that period is still recoverable from the surviving ledger, but deleted tasks' usage is not.
- This replaces the earlier design statement that the package adds no release flag.

## Parallelism

`sequential`

## Inputs

- `.agents/skills/runtime-feature-flags/SKILL.md`.
- [Usage History design](../../specs/costs/system-design/usage-history.md), Purpose and boundaries.

## Results

Added `features.usageHistory` (`KANDEV_FEATURES_USAGE_HISTORY`), restart required, off in prod/dev and on in e2e.
Gated native admission and binding capture, preservation, the migration worker, the Token Usage route, the frontend route/tab, and the retained Host source.
Erasure and task/session completion stay active; completion skips the unretained-copy requirement only while the toggle is off. An off-mode boot records a durable disabled-period marker and reopens the adoption and native cursors; the next enabled boot refreshes the native high-water mark and runs adoption before native migration, so off-period sessions and events catch up.
Tests cover off, on, back on, deletion, erasure, and route absence. The browser toggle spec passes on `chromium` and `mobile-chrome`.

Validation: `go test ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1`, `go test ./internal/backendapp/ -run TestUsageHistoryToggle -count=1`, `go test ./internal/task/repository/sqlite/ -run TestUsageHistory -count=1`, web typecheck, ESLint, statistics Vitest suites, i18n check, and `pnpm e2e:run tests/layout/usage-history-toggle.spec.ts` on `chromium` and `mobile-chrome` passed.
