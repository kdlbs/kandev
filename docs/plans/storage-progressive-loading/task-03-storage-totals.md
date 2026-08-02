---
id: "03-storage-totals"
title: "Storage totals"
status: done
wave: 3
depends_on: ["02-progressive-loading"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 03: Storage totals

## Acceptance

- Storage analysis shows the sum of available non-overlapping top-level measurements and visibly
  identifies a partial total when any required category is unavailable or missing.
- Analysis excludes active/candidate workspace bytes and unused-image bytes because those values
  are subsets of already counted categories.
- Quarantine shows the sum of every currently listed entry's `size_bytes`, including zero/empty
  state, without depending on the overview snapshot.

## Verification

From `apps/`, install dependencies once if absent:

```bash
rtk pnpm install --frozen-lockfile
rtk pnpm --filter @kandev/web test -- components/settings/system/storage/storage-totals.test.ts components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-quarantine-card.test.tsx
rtk pnpm --filter @kandev/web i18n:check
rtk pnpm --filter @kandev/web i18n:ratchet
```

## Files likely touched

- `apps/web/components/settings/system/storage/storage-totals.ts`
- `apps/web/components/settings/system/storage/storage-totals.test.ts`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-quarantine-card.tsx`
- `apps/web/components/settings/system/storage/storage-quarantine-card.test.tsx`
- `apps/web/src/locales/en/settings.json`

## Dependencies

Task 02.

## Parallelism

Sequential. It uses Task 02's loaded-section contract and supplies selectors for Task 04.

## Inputs

- Spec: total-counted and quarantine-total requirements and scenarios
- Plan: Frontend → Totals
- Existing `formatGigabytes` presentation helper

## Risks

- Keep the total a derived UI value; do not persist it or change byte-based API fields.
- Do not translate confirmation tokens, resource IDs, or values used for comparisons.

## Output contract

Report aggregation semantics, localized presentation, files changed, exact test results,
blockers/risks, and update this task plus `plan.md` status in the same conversation.

## Results

- Added pure `storageAnalysisTotal` and `quarantineTotalBytes` helpers. The analysis total adds
  workspace total bytes, quarantine bytes, managed and distinct user Go caches, managed-container
  writable layers, Docker image layers, and Docker build cache; active/candidate workspace bytes
  and unused-image bytes remain excluded subsets.
- Missing, invalid, or unavailable top-level measurements set `partial: true` while preserving all
  valid measurements in the displayed total. Quarantine totals sum every listed entry independently
  of the overview snapshot.
- Added localized total labels and a partial-measurement badge to the analysis card, plus a
  localized quarantine total.
- RED: `storage-totals.test.ts` initially failed because the new helper module did not exist.
- GREEN: `rtk pnpm --filter @kandev/web test -- --run components/settings/system/storage/storage-totals.test.ts components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-quarantine-card.test.tsx` — 3 files, 13 tests passed.
- GREEN: `rtk pnpm --filter @kandev/web i18n:check` passed.
- GREEN: `rtk pnpm --filter @kandev/web i18n:ratchet` passed.
- GREEN: `rtk pnpm run typecheck` from `apps/web` passed.
