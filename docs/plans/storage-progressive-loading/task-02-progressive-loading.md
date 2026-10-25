---
id: "02-progressive-loading"
title: "Progressive section loading"
status: done
wave: 2
depends_on: ["01-policy-endpoint"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 02: Progressive section loading

## Acceptance

- Policy, history, and quarantine publish and render without waiting for the overview request.
- Each section distinguishes loading, empty, ready, and failed states without hiding successful
  sibling sections or accepting stale same-section responses.
- Save, adoption, restore, and terminal-job refreshes request only the data they need and never make
  lightweight section updates wait for an invalidated overview scan.

## Verification

From `apps/`, install dependencies once if absent:

```bash
rtk pnpm install --frozen-lockfile
rtk pnpm --filter @kandev/web test -- --run lib/api/domains/system-api.test.ts lib/state/slices/system/system-slice.test.ts hooks/domains/system/use-storage-maintenance.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-run-history.test.tsx components/settings/system/storage/storage-quarantine-card.test.tsx
```

From `apps/`:

```bash
rtk pnpm --filter @kandev/web run typecheck
```

The focused suite and typecheck run from the `apps/` workspace so pnpm resolves the monorepo
workspace consistently.

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/lib/state/slices/system/types.ts`
- `apps/web/lib/state/slices/system/system-slice.ts`
- `apps/web/lib/state/slices/system/system-slice.test.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-run-history.tsx`
- `apps/web/components/settings/system/storage/storage-run-history.test.tsx`
- `apps/web/components/settings/system/storage/storage-quarantine-card.tsx`
- `apps/web/components/settings/system/storage/storage-quarantine-card.test.tsx`

## Dependencies

Task 01.

## Parallelism

Sequential. It establishes the section state consumed by Tasks 03 and 04.

## Inputs

- Spec: independent loading, isolated failures, and persistence scenarios
- Plan: Frontend → API and state contracts; Independent loading and refresh
- Existing controller generation guard and terminal refresh retry tests

## Risks

- Preserve settings drafts when policy responses arrive; never overwrite an unsaved user edit.
- Avoid turning a section failure into an unhandled promise rejection or a permanent loading state.
- Reconcile these likely files with the actual diff before marking the task done.

## Output contract

Report section state behavior, mutation refresh behavior, files changed, exact test results,
blockers/risks, and update this task plus `plan.md` status in the same conversation.

## Results

- Added the lightweight `GET /api/v1/system/storage/settings` client contract and persisted policy
  state separately from the scan-backed overview.
- Split storage loading into policy, overview, history, and quarantine sections with per-section
  loading/error state and generation guards so stale responses cannot overwrite newer data.
- Policy save/adoption and quarantine restore now refresh only their affected sections; terminal job
  refreshes still reload all sections for a consistent post-run snapshot.
- Stale section failures no longer escape newer successful reloads, and Go-cache adoption invalidates
  in-flight policy reads before committing its response.
- Rendered independent loading/error cards for policy, history, and quarantine while overview scanning
  remains in progress.
- RED: API/store-focused tests initially failed because the new policy client and setter were absent.
- GREEN: `rtk pnpm --filter @kandev/web test -- --run lib/api/domains/system-api.test.ts lib/state/slices/system/system-slice.test.ts hooks/domains/system/use-storage-maintenance.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-run-history.test.tsx components/settings/system/storage/storage-quarantine-card.test.tsx` — 7 files, 85 tests passed.
- GREEN: `rtk pnpm --filter @kandev/web run typecheck` from `apps` passed.
