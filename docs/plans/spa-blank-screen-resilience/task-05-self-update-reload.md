---
id: "05-self-update-reload"
title: "Reload after confirmed self-update"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/system-page.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 05: Reload After Confirmed Self-Update

## Acceptance

- RED tests distinguish document reload from the existing update-metadata
  refetch.
- `UpdatesCard` passes a stable, injectable document-reload callback to
  `useSelfUpdate`.
- Reload occurs only after `/system/info` equals the target version; older
  versions, unreachable backend states, and failed jobs do not reload.
- Persisted self-update state is cleared before completion.
- Overlapping poll ticks can invoke completion and document reload at most once.
- Desktop-native update behavior and the existing progress/error UI remain
  unchanged.

## Files likely touched

- `apps/web/components/settings/system/updates-card.tsx`
- `apps/web/components/settings/system/updates-card.test.tsx`
- `apps/web/hooks/domains/system/use-self-update.ts`
- `apps/web/hooks/domains/system/use-self-update.test.ts`

## Verification

```bash
cd apps/web
pnpm test -- hooks/domains/system/use-self-update.test.ts components/settings/system/updates-card.test.tsx
pnpm run typecheck
```

## Output contract

Report the authoritative version-confirmation seam, exactly-once guard,
document-reload injection, RED/GREEN results, and files changed. The primary
session updates this task and `plan.md` after accepting the result.
