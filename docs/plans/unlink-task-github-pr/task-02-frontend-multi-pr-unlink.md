---
id: "02-frontend-multi-pr-unlink"
title: "Frontend multi-PR unlink UI"
status: done
wave: 2
depends_on: ["01-backend-unlink-contract"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/link-existing-task-github-issue.md"
---

# Task 02: Frontend Multi-PR Unlink UI

## Acceptance

- Every tab in a multi-PR desktop popover and mobile drawer has an accessible
  unlink button; the label button and close button are valid sibling controls,
  and the mobile target is at least 44px.
- Successful unlink removes only the selected association from local state and
  live updates do the same in other clients; failure retains the tab and shows
  an error.
- Tab selection, roving keyboard navigation, pending state, adjacent fallback,
  and the two-to-one PR surface collapse remain deterministic.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/github/pr-ci-popover.automation.test.tsx lib/api/domains/github-api.test.ts lib/state/slices/github/github-slice.test.ts lib/ws/handlers/github.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/github.ts`
- `apps/web/lib/ws/handlers/github.test.ts`
- `apps/web/hooks/domains/github/use-task-pr.ts`
- `apps/web/components/github/multi-pr-ci-popover.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/components/github/pr-topbar-button.tsx`
- `apps/web/components/github/pr-status-chip.tsx`

## Dependencies

Task 01's HTTP and WebSocket contracts.

## Parallelism

Sequential. This task consumes Task 01's exact payload and endpoint shapes and
owns shared GitHub frontend state used by the E2E task.

## Inputs

- Spec scenarios: `Unlink one of several pull requests`, `Keep an association
  when unlink fails`, `Reach unlink from a touch device`.
- Plan sections: `API, store, and live updates`, `Multi-PR tab close action`,
  `Mobile design contract`.
- Patterns: GitLab `MRTopbarButton` unlink error handling and the existing
  `PRStatusChipMultiDrawer` touch composition.

## Risks

- Do not nest an icon button inside the existing `role="tab"` button.
- A successful two-to-one removal can unmount the active multi-PR component;
  focus logic must not assume the tablist still exists.

## Output contract

Report the RED/GREEN evidence, files changed, exact command results, mobile
interaction details, remaining risks/blockers, and update this task plus
`plan.md` status in the same conversation.
