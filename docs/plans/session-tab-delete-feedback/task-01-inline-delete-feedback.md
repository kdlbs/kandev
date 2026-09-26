---
id: "01-inline-delete-feedback"
title: "Inline session delete feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/session-tab-delete-feedback.md"
---

# Task 01: Inline session delete feedback

## Intent

Move X-initiated agent-session deletion feedback into the tab close control without changing the
confirmation, context-menu feedback, mobile feedback, or successful session cleanup behavior.

## Acceptance

- Confirmed X-originated deletion renders a disabled, busy spinner in place of the X and emits no
  progress or success toast; repeated activation cannot dispatch another request.
- A failed X-originated deletion keeps the tab/session, restores the X, and emits exactly one error
  toast. Default session-action callers retain their current toast sequence.
- Successful deletion still hands off the active session before removing local session state and
  the Dockview panel.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-actions.ts`
- `apps/web/hooks/domains/session/use-session-actions.test.ts`
- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/session-tab-close-action.tsx`
- `apps/web/components/task/session-tab-close-action.test.tsx`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

None.

## Parallelism

Sequential. The hook contract, tab integration, and close-action state are one behavior and share
the same focused tests.

## Inputs

- Spec: `What`, `Failure modes`, and the first four `Scenarios`.
- Plan: `Shared session delete action`, `Session tab close action`, and `Tests`.
- Existing patterns: `useSessionActions` success-only cleanup ordering,
  `SessionTabTriggerContent`, `shouldMarkSessionTabUserActivationIntent`, and `GridSpinner` in
  `apps/web/components/enhance-prompt-button.tsx`.

## Verification

Bootstrap once if this worktree does not already have dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run:

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-session-actions.test.ts components/task/session-tab-close-action.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

## Output contract

Report the feedback-mode API, close-control behavior, files changed, exact test results, blockers,
risks, and synchronized task/plan status. Do not change backend deletion or other lifecycle-action
feedback.

## Results

- Added the optional `remove({ feedback })` contract. Default callers retain loading/success/error
  toasts; the tab-X mode suppresses progress/success and emits one error toast on failure while
  preserving success-only store/panel cleanup.
- Replaced Dockview's opaque session close icon with a localized repository-owned X/spinner action
  that preserves the close test ID and activation guard. Confirmation origin tracking limits the
  inline behavior to tab-X deletes; context-menu and mobile callers remain on toast feedback.
- Added deterministic idle/pending close-action coverage, including disabled state, `aria-busy`, and
  blocked repeat activation.
- `rtk pnpm --filter @kandev/web test -- components/task/session-tab-close-action.test.tsx hooks/domains/session/use-session-actions.test.ts` — 2 files, 15 tests passed.
- `rtk pnpm run typecheck` — passed.
- `rtk pnpm run i18n:check` — passed with pseudo locale in sync (the existing 670 zh-cn parity
  notices are advisory).
- `rtk pnpm run i18n:ratchet` — passed.
- `rtk pnpm exec eslint components/task/session-tab.tsx components/task/session-tab-close-action.tsx hooks/domains/session/use-session-actions.ts hooks/domains/session/use-session-actions.test.ts components/task/session-tab-close-action.test.tsx e2e/tests/session/session-tab-management.spec.ts e2e/tests/session/mobile-session-deletion.spec.ts` — passed.
- `rtk git diff --check` — passed.

Generated artifact: `apps/web/src/locales/pseudo/common.json`. No external side effects beyond local
WebSocket test mocks.

## Localized-confirmation follow-up

The later shipped refinement preserves this task's tab-X dialog and feedback-mode contract while
moving desktop context-menu confirmation into an anchored popover and phone confirmation into its
Sessions picker row. Shared warning copy now lives in the purpose-neutral
`components/task/session-delete-description.tsx`; the context-menu event and `preventDefault()`
contracts are documented beside their public callback and Radix handler.
