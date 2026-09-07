---
id: "01-shared-confirmation-shell"
title: "Shared confirmation shell"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
acceptance_criteria:
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.2
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.3
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.4
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.6
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.7
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.8
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.10
system_design:
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
---

# Task 01: Shared Confirmation Shell

## Summary

Implement one presentation-only confirmation shell for named saved task views.
It adapts the existing anchored and inline primitives, centralizes copy and
boundary handling, and invokes a captured target ID only after confirmation.

## In scope

- Add `SavedTaskViewDeleteConfirmation` and its target/prop contract.
- Support fine-pointer popover and touch-density inline presentations.
- Reuse the existing confirmation-boundary marker and provide a shared target
  predicate for nested parent surfaces.
- Add localized title, consequence/reassurance, Cancel, Delete, and accessible
  names in all supported catalogs.
- Test cancellation, Escape, focus, stale anchors, pending confirmation, and
  exactly-once target dispatch.

## Out of scope

- Wiring any saved-view surface or calling a store/provider mutation.
- Changing `ActionConfirmPopover` or `InlineConfirmActions` behavior beyond the
  small boundary adapter required by this contract.
- Adding persistence, retry, or error state.

## Acceptance

- Opening either presentation performs no mutation; Cancel, Escape, dismissal,
  and a disconnected anchor preserve the target and restore focus when possible.
- The visible and accessible confirmation names the supplied label, uses a
  destructive final action, and dispatches the captured ID once after close.
- Fine-pointer content exposes a nested-overlay boundary; inline content uses
  44px touch actions and contains long localized text.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- \
  components/confirmation/saved-task-view-delete-confirmation.test.tsx \
  components/confirmation/action-confirm-popover.test.tsx
cd web
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
pnpm run i18n:check
pnpm run typecheck
```

## Files likely touched

- `apps/web/components/confirmation/saved-task-view-delete-confirmation.tsx`
- `apps/web/components/confirmation/saved-task-view-delete-confirmation.test.tsx`
- `apps/web/components/confirmation/action-confirm-popover.tsx`
- `apps/web/components/confirmation/action-confirm-popover.test.tsx`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pt-pt/common.json`
- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/zh-hk/common.json`
- `apps/web/src/locales/zh-tw/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

None.

## Risks

- A generic boundary predicate must not treat unrelated portal content as part
  of the confirmation.
- `ActionConfirmPopover` and `InlineConfirmActions` close at different DOM
  locations; the adapter must preserve both primitives' existing focus rules.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria `.2`, `.3`, `.4`, `.6`, `.7`, `.8`, and `.10`.
- System-design sections Shared confirmation shell, Data and contracts, Control
  flow, and Responsive and accessibility behavior.
- Existing `WatcherDeleteAction`, `SavedLayoutDeleteConfirmation`,
  `ActionConfirmPopover`, and `InlineConfirmActions` patterns.

## Results

- Installed the frozen pnpm workspace dependencies for this worktree.
- Added the shared fine-pointer/inline shell, captured-ID dispatch, and nested
  confirmation-boundary predicate.
- Added synchronized `common` copy in all real locales and the pseudo locale;
  `i18n:check` passed.
- Focused confirmation suites passed 15/15 and web typecheck passed.
