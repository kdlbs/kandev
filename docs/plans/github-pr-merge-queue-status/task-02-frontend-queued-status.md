---
id: "02-frontend-queued-status"
title: "Frontend queued PR status"
status: done
wave: 2
depends_on: ["01-backend-queue-state"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-pr-merge-queue.md"
---

# Task 02: Frontend queued PR status

## Acceptance

- Open queued PRs use GitHub's `#966600` merge-queue color for the task icon and
  compact chip. Yellow remains the CI-in-progress color.
  terminal PRs and more attention-worthy multi-PR siblings retain their
  established priority.
- The task hover summary, compact desktop popover or phone drawer, and PR detail
  notice show localized queue-state copy, one-based position, and the available
  estimated merge duration without exposing raw GitHub values.
- All five locale catalogs, i18n checks, typecheck, and focused component tests
  cover queued, queue-exit, unknown-enum, and terminal behavior.
- Terminal colors remain authoritative, then an active queue entry takes
  precedence over other non-terminal failure, draft, dirty, or behind fields.
  A failing sibling still outranks a queued sibling during multi-PR
  aggregation, and future provider states use generic queued copy.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/github/pr-task-icon.test.ts components/github/pr-task-status-summary.test.ts components/github/pr-status-chip.test.tsx components/github/pr-merge-queue-status.test.tsx components/github/pr-mergeability-row.test.tsx components/github/pr-detail-panel.test.tsx && pnpm --filter @kandev/web i18n:check && pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/lib/types/github.ts`
- `apps/web/components/integrations/change-request-task-status-color.ts`
- `apps/web/components/integrations/change-request-task-status-summary.tsx`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-status-summary.tsx`
- `apps/web/components/github/pr-status-chip.tsx`
- `apps/web/components/github/pr-merge-queue-status.tsx`
- `apps/web/components/github/pr-mergeability-row.tsx`
- `apps/web/components/github/pr-detail-panel.tsx`
- `apps/web/components/github/pr-task-icon.test.ts`
- `apps/web/components/github/pr-task-status-summary.test.ts`
- `apps/web/components/github/pr-status-chip.test.tsx`
- `apps/web/components/github/pr-merge-queue-status.test.tsx`
- `apps/web/components/github/pr-mergeability-row.test.tsx`
- `apps/web/components/github/pr-detail-panel.test.tsx`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pt-pt/github.json`
- `apps/web/src/locales/zh-cn/github.json`
- `apps/web/src/locales/zh-hk/github.json`
- `apps/web/src/locales/zh-tw/github.json`

## Dependencies

Task 01 supplies the normalized TaskPR queue-entry contract and bounded `queued`
aggregate.

## Parallelism

Sequential. It consumes the backend contract and owns shared frontend status
helpers used by both E2E surfaces.

## Inputs

- Spec sections: **What**, **State Machine**, queue-status scenarios, and **Out
  of Scope**.
- Plan sections: **Types and semantic color**, **Hover, compact status, and
  review detail**, and **Mobile design contract**.
- Existing patterns: `getPRStatusColor`, `derivePRTaskStatusSummary`,
  `PRMergeabilityRow`, `PRMergeabilityNotice`, and `PRStatusChipDrawer`.

## Risks

- Queue color precedence must not change merged or closed colors. Tests must
  distinguish the exact `#966600` queue color from yellow CI-in-progress.
- The optional estimate must not suppress queue state or position when GitHub
  returns it as null.
- Queue copy must remain available on coarse pointers through the drawer and
  Review surface; a tooltip-only result would violate mobile parity.

## Output contract

Report the summary, exact files changed, exact commands and results, generated
locale changes, blockers, remaining risks, rendered verification evidence, and
synchronized task/plan status in this conversation.

## Results

Passed:

- Implemented the queued semantic, `#966600` color, localized queue metadata formatter, task summary detail row, compact status chip, drawer, and PR detail notice.
- `cd apps/web && pnpm test -- components/github/pr-task-icon.test.ts components/github/pr-task-status-summary.test.ts components/github/pr-status-chip.test.tsx components/github/pr-merge-queue-status.test.tsx components/github/pr-mergeability-row.test.tsx components/github/pr-detail-panel.test.tsx` passed 160 tests across 6 files.
- `cd apps/web && pnpm run typecheck` passed. `cd apps/web && pnpm run lint` passed with no warnings or errors. `cd apps/web && pnpm run i18n:check` passed with 7,223 referenced keys, 8,779 English entries, 48 orphans, and complete `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw` catalogs.
- Added queue keys to English, Portuguese, Simplified Chinese, Traditional Chinese, and pseudo catalogs. `pnpm run i18n:zh-hant` reached its existing unrelated `agents:dynamicProfileSettings` residual; the namespace-specific `convert-zh-cn-to-zh-hant.mjs --locale all --namespace github --write` command generated the Traditional Chinese queue entries with zero residual queue keys. `pnpm run i18n:pseudo` generated the pseudo queue entries.
- Desktop and mobile E2E runs rendered the queue state, position, and estimate in the two display scenarios, while the two action scenarios restored API, notification, duplicate-suppression, and mobile target coverage. The reviewed files were `apps/web/.pr-assets/pr-merge-queue--desktop-pr-merge-queue-status.png` and `apps/web/.pr-assets/mobile-pr-merge-queue--mobile-pr-merge-queue-status.png`; both were removed by the final `pnpm e2e:clean` run after preservation for PR media publication.
- No external side effects occurred. The implementation reads GitHub queue metadata and does not add queue-management actions.
- Queue precedence regressions pass for a queued PR with failure, changes
  requested, dirty, behind, or draft fields, while failing siblings retain
  aggregate priority. Future provider enums resolve to `queue_queued` and the
  generic localized `Queued` label.
