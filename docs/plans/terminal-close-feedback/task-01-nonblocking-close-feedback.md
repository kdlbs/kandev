---
id: "01-nonblocking-close-feedback"
title: "Optimistic terminal dismissal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/terminal-close-feedback.md"
---

# Task 01: Optimistic terminal dismissal

## Intent

Keep confirmation local to its close control, then make it the terminal's final visible state while
teardown continues in the background.

## Acceptance

- Busy and script terminals retain confirmation; cancel does nothing.
- Desktop and tablet render a compact anchored popover without a page backdrop or blocking modal.
- Confirming removes the popover, local terminal state, active content, and Dockview panel before
  the destroy request settles.
- A ref-backed guard suppresses duplicate requests without rendering pending state.
- Success is silent. Failure leaves the terminal dismissed and emits one localized error toast.

## Files

- `apps/web/components/task/close-terminal-confirm-popover.tsx`
- `apps/web/components/task/terminal-tab.tsx`
- `apps/web/hooks/domains/session/use-terminal-destroy.ts`
- `apps/web/hooks/domains/session/use-terminals.ts`
- Focused tests alongside those files

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run components/task/close-terminal-confirm-popover.test.tsx components/task/terminal-tab.test.tsx hooks/domains/session/use-terminal-destroy.test.ts
```

## Results

- Popover and terminal removal now precede the unresolved destroy transport.
- Confirmation remains beside the initiating close control and never blocks the page.
- Dockview renders no closing spinner or busy state.
- Duplicate work remains synchronously guarded; background failures toast once without rollback.
- Included in the final focused run: 4 files, 12 tests passed.
