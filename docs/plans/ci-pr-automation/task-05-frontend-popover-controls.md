---
id: "05-frontend-popover-controls"
title: "Frontend popover controls"
status: done
wave: 3
depends_on: ["04-frontend-api-state-hook"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 05: Frontend Popover Controls

## Acceptance

- `PRCIPopover` renders auto-fix and auto-merge controls for open linked PRs on desktop and mobile.
- Users can edit and reset the task-specific auto-fix prompt without hover/drawer layout breakage.
- The automation section includes an info icon/help affordance that explains watch cadence, watched states, snapshots/dedupe, and merge gates.
- The task prompt editor includes a link to Settings > Prompts for editing the default `ci-auto-fix` prompt.
- Component tests cover rendering, toggling, help content, prompt override save/reset, settings link, disabled states, and mobile layout basics.

## Verification

```bash
cd apps && rtk pnpm --filter @kandev/web test -- pr-automation-controls
cd apps && rtk pnpm --filter @kandev/web test -- pr-ci-popover
cd apps && rtk pnpm --filter @kandev/web typecheck
```

## Files Likely Touched

- `apps/web/components/github/pr-ci-popover.tsx`
- `apps/web/components/github/multi-pr-ci-popover.tsx`
- `apps/web/components/github/pr-status-chip.tsx`
- `apps/web/components/github/pr-automation-controls.tsx`
- `apps/web/components/github/pr-automation-controls.test.tsx`
- `apps/web/components/github/pr-ci-popover.test.tsx`
- `apps/web/components/github/pr-status-chip.test.tsx`
- `apps/web/components/settings/prompts-settings.tsx` only if the new built-in prompt needs explicit UI affordances

## Dependencies

- `04-frontend-api-state-hook`

## Inputs

- Spec sections: What, Scenarios, Failure modes.
- Plan sections: Frontend > Popover controls; Frontend > Prompt settings.
- Existing patterns: `PRMergeButton`, `usePRCIPopover`, shadcn components from `@kandev/ui`, and mobile drawer behavior in `PRStatusChip`.
- UI requirements: use a compact icon button for help and an edit icon/button for the task prompt editor; provide accessible labels/tooltips for icon-only controls.
- Frontend guidance: use mobile parity checks for this UI change.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 3 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
