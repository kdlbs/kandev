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

- Create a stable, identity-scoped QueryClient in the authenticated app branch.
- Fetch `/api/v1/system/info` lazily through the existing API wrapper and keep explicit refetch, loading, error, and one-attempt behavior.
- Remove only the SystemInfo field and action from the Zustand System slice. Keep all other System state and actions there.
- Keep the boot payload and `runtime.bootId` unchanged. Keep backend-generation, restart, and self-update reads on their independent no-store paths.
- Add real provider/hook tests for deduplication, freshness, retry, explicit refresh, identity changes, cancellation, and stale-response isolation.

## Exclusions

- New endpoints, boot-payload snapshots, product behavior, or authentication boundaries.
- Migration of database, jobs, storage, metrics, backups, or other System resources.
- Changes to restart/update control probes, WebSocket policy, or shared HTTP transport.
- A generic WebSocket-to-Query bridge or a query-owner lint rule.

## Acceptance

1. Preserve `AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.1` and `.2`: use the current page boot ID and retain an independent uncached reconnect check.
2. Query owns the About view's single SystemInfo snapshot and request state. Its key and client scope include the canonical full API base URL, page boot ID, auth mode, authenticated state, and user ID.
3. About mounts trigger one deduplicated request. Loading, error, manual retry/refetch, and the immutable process snapshot keep the current UI behavior.
4. Identity changes and provider teardown abort pending requests and discard that identity's cache. A late response cannot appear in the replacement cache.
5. All unrelated System state remains in Zustand. No SystemInfo result is copied back into Zustand.

## Verification

```sh
cd apps/web && pnpm exec vitest run hooks/domains/system/use-system-info.test.tsx lib/state/slices/system/system-slice.test.ts hooks/domains/system/use-backend-generation-guard.test.ts hooks/domains/system/use-kandev-restart.test.ts hooks/domains/system/use-self-update.test.ts lib/api/domains/system-api.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps && pnpm install --frozen-lockfile
python3 scripts/lint-architecture.py --all
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py && python3 .github/scripts/lint-harness-files.py --all
cd apps/web && pnpm run i18n:ratchet
```

No Playwright E2E was warranted because the change does not alter About layout, copy, controls, or responsive behavior.

## Results

Implementation and verification are complete. The linked system designs remain unchanged except for the new scoped cache design; no product requirement was added.
