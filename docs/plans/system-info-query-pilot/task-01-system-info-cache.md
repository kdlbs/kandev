---
id: "01-system-info-cache"
title: "Move About SystemInfo to the scoped Query cache"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.1
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.2
system_design:
  - ../../specs/platform/system-design/system-info-query-cache.md
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
---

# Task 01: Move About SystemInfo to the scoped Query cache

## Outcome

Give the About view one authoritative server snapshot and request lifecycle in TanStack Query while preserving its current displayed information and fetch behavior.

## In scope

- Create one stable QueryClient in the authenticated app branch. Keep its
  descendants mounted across identity changes, while cancelling and removing
  only SystemInfo queries from obsolete identities. Call targeted cancellation
  and removal in the same identity-change effect so cleanup cannot outlive a
  later A-to-B-to-A switch.
- Fetch `/api/v1/system/info` lazily through the existing API wrapper and keep explicit refetch, loading, error, and one-attempt behavior.
- Remove only the SystemInfo field and action from the Zustand System slice. Keep all other System state and actions there.
- Keep the boot payload and `runtime.bootId` unchanged. Keep backend-generation, restart, and self-update reads on their independent no-store paths.
- Add real provider/hook tests for concurrent-consumer deduplication, StrictMode
  observer replay, freshness, retry, explicit refresh, logout/backend identity
  changes, cancellation, and stale-response isolation.

## Exclusions

- New endpoints, boot-payload snapshots, product behavior, or authentication boundaries.
- Migration of database, jobs, storage, metrics, backups, or other System resources.
- Changes to restart/update control probes, WebSocket policy, or shared HTTP transport.
- A generic WebSocket-to-Query bridge or a query-owner lint rule.

## Acceptance

1. Preserve `AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.1` and `.2`: use the current page boot ID and retain an independent uncached reconnect check.
2. Query owns the About view's single SystemInfo snapshot and request state. Its key includes the canonical full API base URL, page boot ID, auth mode, authenticated state, and user ID. A stable app-branch client removes obsolete SystemInfo identities in the identity-change effect without remounting unrelated shell state or leaving a stale removal promise pending.
3. About mounts trigger a lazy request; concurrent consumers with the same identity share it. Loading, error, manual retry/refetch, and the immutable process snapshot keep the current UI behavior.
4. Query functions consume TanStack's observer signal. When an identity change
   or logout removes the last observer from an in-flight query, TanStack
   cancels it. Identity changes immediately remove prior SystemInfo entries
   from the shared client using targeted QueryClient calls, and late responses
   cannot appear under a new identity. A deferred cleanup regression covers a
   rapid A-to-B-to-A switch. No exactly-once request promise is made across
   StrictMode observer replay.
5. All unrelated System state remains in Zustand. No SystemInfo result is copied back into Zustand.

## Verification

```sh
cd apps/web && pnpm exec vitest run src/app-error-boundary.test.tsx components/state-provider.test.tsx hooks/domains/system/use-system-info.test.tsx lib/state/slices/system/system-slice.test.ts hooks/domains/system/use-backend-generation-guard.test.ts hooks/domains/system/use-kandev-restart.test.ts hooks/domains/system/use-self-update.test.ts lib/api/domains/system-api.test.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-docker-build-permissions.spec.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps && pnpm install --frozen-lockfile
python3 scripts/lint-architecture.py --all
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py && python3 .github/scripts/lint-harness-files.py --all
cd apps/web && pnpm run i18n:ratchet
```

## Results

The provider keeps one client for the authenticated app branch, scopes the query
key to the full backend/boot/auth identity, and cancels then immediately removes
obsolete SystemInfo entries without remounting unrelated shell state. It does
not defer cache removal until cancellation settles. The boot payload and backend
remain unchanged.

The focused SystemInfo, error-boundary, state-provider, System slice, restart
guard, restart flow, self-update, and System API suite passes (81 tests across
8 files). It also proves an edited non-query child keeps its state through
identity changes and that delayed cancellation completion cannot remove the
returned identity's active cache entry. Typecheck, lint, frozen-lockfile
install, architecture lint, docs validation, spec lint, and i18n ratchet pass.
The managed mobile Docker permissions E2E passes both member and admin cases.
