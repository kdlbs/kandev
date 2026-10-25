---
id: "02-shared-watcher-saved-baseline"
title: "Reconcile shared watcher saved state"
status: done
wave: 2
depends_on: ["01-github-review-watch-response"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-manual-save.md"
---

# Task 02: Reconcile shared watcher saved state

## Acceptance

- A successful watcher enabled-state save becomes the clean rendered baseline
  even when the incoming integration list still contains the old value.
- An edit made while Save is in flight remains visible and dirty after the
  earlier request succeeds.
- The temporary saved baseline is removed after authoritative items match it or
  the watcher disappears.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/integrations/use-watcher-enabled-drafts.test.tsx
```

## Files likely touched

- `apps/web/components/integrations/use-watcher-enabled-drafts.ts`
- `apps/web/components/integrations/use-watcher-enabled-drafts.test.tsx`

## Dependencies

Task 01 documents the authoritative endpoint contract the hook reconciles
against.

## Parallelism

Sequential.

## Inputs

- `docs/specs/ui/requirements/settings-manual-save.md`
- `docs/plans/integration-watcher-save-state/plan.md`
- `docs/decisions/0046-settings-route-save-coordinator.md`

## Risks

React may batch the saved-baseline and draft updates. The test must assert the
user-visible state, not update ordering or setter calls.

## Output contract

Report the red and green command results, files changed, blockers, risks, and
update this task plus `plan.md` status in the primary conversation.
