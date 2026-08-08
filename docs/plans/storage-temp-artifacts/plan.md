---
spec: docs/specs/system-page/storage-maintenance.md
created: 2026-08-08
status: complete
---

# Implementation Plan: Registered Kandev temporary-artifact cleanup

## Overview

Add a manual-only `temporary_artifacts` resource to the existing Storage page. The backend first
gets a durable registry and marker contract for exact Kandev service-created temporary roots, then
adds analysis/quarantine/API composition, followed by the responsive resource action and desktop /
mobile coverage. The implementation must preserve the inherited shared-temp boundary: anonymous
`/tmp` folders, caches, preview/CI roots, and other-installation data are never swept.

The architectural boundary is recorded in
[ADR-2026-08-08-owned-temp-artifact-cleanup](../../decisions/2026-08-08-owned-temp-artifact-cleanup.md),
which supplements [ADR 0045](../../decisions/0045-install-wide-storage-maintenance.md).

---

## Backend

### Ownership registry and producer lifecycle

- Add a `storage_temp_artifacts` table and replay-safe migration in
  `apps/backend/internal/system/storage/store_migrations.go`, with exact path, allowlisted kind,
  artifact ID, marker token, lifecycle state, owner PID/lease timestamps, and metadata.
- Add `apps/backend/internal/system/storage/tempartifacts/` with registry operations that create
  top-level directories below the effective service temp root, write an owner-only marker atomically,
  persist the record, heartbeat active leases, close normal lifecycles, and reconcile records after
  restart. All path checks use `Lstat`/canonical containment and fail closed on symlinks or
  uncertainty.
- Migrate backend-owned long-lived roots in `apps/backend/internal/improvekandev/bundle.go` and
  `apps/backend/internal/agent/hostutility/manager.go` to the registry. Remove the independent
  name/age sweep from `apps/backend/internal/backendapp/helpers.go`; unregistered legacy roots stay
  host-policy data.

### Storage provider and API composition

- Implement analysis and explicit cleanup in `tempartifacts/provider.go`: report total, active,
  stale-candidate, skipped, and unavailable bytes/counts; only closed or abandoned records older
  than 24 hours qualify; same-filesystem rename moves a qualifying root to the existing quarantine.
- Extend storage summary/capability types and `storage_quarantine_entries.resource_type` for
  `temporary_artifact`. Reuse quarantine retention, restore, and permanent deletion; do not add a
  `storage_maintenance` setting in this release.
- Reuse `ExplicitCleanupProvider` so `temporary_artifacts` is no-op for scheduled and unscoped
  full runs but performs cleanup for `resources: ["temporary_artifacts"]`. Extend provider
  composition in `apps/backend/internal/backendapp/storage_maintenance.go` and keep overview totals
  non-overlapping.
- Accept the stable resource name through the existing `POST /api/v1/system/storage/run` contract;
  return provider counts/bytes and skipped warnings through the existing run result and preserve
  the current activity gate and busy/force behavior.

---

## Frontend

### Storage analysis and action

- Extend `apps/web/lib/types/system.ts` with the temporary-artifact summary, capability, quarantine
  resource type, and provider result shapes. No new policy field is added.
- Update `apps/web/components/settings/system/storage/storage-totals.ts` so measured temporary
  artifact bytes participate once in the non-overlapping total, with a partial result when that
  provider is unavailable.
- Add a localized **Kandev temporary artifacts** resource row to
  `apps/web/components/settings/system/storage/storage-overview-card.tsx`. Show measured bytes,
  stale eligible bytes/count, active protection, skipped warnings, and an explicit
  **Clean stale artifacts** action only when cleanup is available.
- Thread `runNow(["temporary_artifacts"])` through the existing Storage controller and add a normal
  reversible confirmation in `storage-confirmation-dialogs.tsx`; do not introduce a second page or
  a scheduled toggle. Keep busy feedback and `Run anyway` on the existing gate.
- Add the new copy to `en`, `pseudo`, `pt-pt`, and `zh-cn` `system.json` catalogs. All paths remain
  i18n-guarded and filesystem values are interpolated rather than translated.

### Mobile contract

- Keep the existing one-column Storage accordion and action composition. The temporary-artifact row
  must wrap long paths/warnings, retain a 44px-or-larger action target, and remain usable with no
  horizontal document overflow at the mobile project viewport.

---

## Tests

- **Registry persistence and marker safety:** create/close/heartbeat/reconcile registered artifacts,
  reject unregistered prefixes, missing or mismatched markers, symlink replacement, path escape, and
  cross-install/legacy records. **Files:**
  `apps/backend/internal/system/storage/tempartifacts/*_test.go`,
  `apps/backend/internal/system/storage/store_test.go`,
  `apps/backend/internal/improvekandev/bundle_test.go`, and host-utility lifecycle tests.
  **How:** table-driven filesystem tests under `t.TempDir()` plus SQLite migration/restart tests.
- **Provider behavior:** analysis is read-only, active/recent roots are protected, stale roots move
  atomically to quarantine, cross-device/error/symlink cases remain untouched, and unscoped/scheduled
  runs do not invoke destructive temp cleanup. **Files:**
  `apps/backend/internal/system/storage/tempartifacts/provider_test.go`,
  `apps/backend/internal/system/storage/operations_test.go`, and
  `apps/backend/internal/system/storage/runner_test.go`.
  **How:** fake clocks, fake leases, fake rename/filesystem adapters, and provider result assertions.
- **API and composition:** the overview exposes the new summary, `temporary_artifacts` is accepted
  only as a valid explicit resource, quarantine rows round-trip the new resource type, and handler
  errors preserve `400`/`409` semantics. **Files:**
  `apps/backend/internal/system/storage/handler_test.go`, `storage_routes_test.go`, and
  `apps/backend/internal/backendapp/storage_test.go`.
  **How:** handler-to-service-to-store tests with real SQLite and a fake provider, plus composition
  wiring coverage.
- **Frontend rendering/action:** the row shows active versus stale state, the confirmation sends the
  exact resource selection, success/error/busy feedback remains localized, and existing Go-cache /
  global actions remain unchanged. **Files:**
  `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`,
  `storage-confirmation-dialogs.test.tsx`,
  `storage-totals.test.ts`,
  `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`, and
  `apps/web/lib/api/domains/system-api.test.ts`.
  **How:** Vitest and Testing Library interaction tests with mocked overview/run responses.

---

## E2E Tests

- **Scenario:** a current-install registered stale artifact and an unregistered `kandev-*` folder
  are present, **WHEN** Storage analysis runs, **THEN** only the registered artifact appears in the
  temporary-artifact row and the unregistered folder is not counted or changed. **File:**
  `apps/web/e2e/tests/system/storage-maintenance.spec.ts` using the existing Storage API route
  fixture pattern plus an explicit-run assertion.
- **Scenario:** the user selects **Clean stale artifacts**, **WHEN** confirmation completes,
  **THEN** the request contains `{ resources: ["temporary_artifacts"] }`, the cleanup job completes,
  and the result is visible in Quarantine while a global **Run now** leaves the artifact untouched.
  **File:** `apps/web/e2e/tests/system/storage-maintenance.spec.ts`.
- **Scenario:** the same row and confirmation are opened from the mobile settings sheet, **WHEN**
  the user taps the action, **THEN** the action is reachable at a 44px target and the document has no
  horizontal overflow. **File:** `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`.

Focused UI commands use the managed production-build runner:

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
```

---

## Verification Results

- Backend storage, Improve Kandev, host-utility, and backend-app tests passed:
  `go test ./internal/system/storage/... ./internal/improvekandev/... ./internal/agent/hostutility/... ./internal/backendapp/... -count=1`.
- Focused frontend tests passed: 71 tests across six files; web typecheck and lint passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. The check reported only the
  repository's existing 209 advisory pt-pt/zh-cn parity issues and one existing orphan.
- Desktop Storage Playwright: 7 passed, including the registered-artifact row, scoped cleanup,
  quarantine/job feedback, cache isolation, busy override, progressive loading, and settings
  persistence scenarios.
- Mobile Storage Playwright: 5 passed, including the scoped action, touch target, and document
  overflow assertions.
- Public documentation validation passed: 58 tests and 41 published pages. `git diff --check`
  passed.

---

## Implementation Waves And Parallel Candidates

Wave 1 (sequential backend foundation):

- [x] [Task 01 — Temp ownership registry and producer lifecycle](task-01-temp-ownership-registry.md)

Wave 2 (backend provider/API after Task 01):

- [x] [Task 02 — Temporary-artifact storage provider and API](task-02-temp-artifact-provider-api.md)

Wave 3 (frontend after Task 02; shared frontend files make this sequential):

- [x] [Task 03 — Storage temporary-artifact action](task-03-storage-temp-artifact-ui.md)

Wave 4 (desktop/mobile E2E after Task 03):

- [x] [Task 04 — Temporary-artifact Storage E2E coverage](task-04-storage-temp-artifact-e2e.md)

Wave 5 (docs after behavior is implemented):

- [x] [Task 05 — Temporary-artifact operations documentation](task-05-storage-temp-artifact-docs.md)
