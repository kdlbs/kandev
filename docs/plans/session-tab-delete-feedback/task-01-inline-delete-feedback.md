---
id: "01-inline-delete-feedback"
title: "Non-destructive session tab closing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001
acceptance_criteria:
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.1
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.2
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.3
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.4
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.5
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.6
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.7
  - AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.8
system_design:
  - ../../specs/ui/system-design/session-tab-close-and-delete.md
---

# Task 01: Non-destructive session tab closing

## Intent

Make the desktop Agent tab's X, Hide, and Close Others remove only Dockview panels, keep every
conversation recoverable from **+ > Agents**, keep deletion an explicit confirmed action, and keep
the per-environment hide record durable across reload without changing session lifecycle
semantics or the mobile Sessions picker.

## Acceptance

- Confirmed X-originated deletion renders a disabled, busy spinner in place of the X and emits no
  progress or success toast; repeated activation cannot dispatch another request.
- A failed X-originated deletion keeps the tab/session, restores the X, and emits exactly one error
  toast. Default session-action callers retain their current toast sequence.
- Successful deletion still hands off the active session before removing local session state and
  the Dockview panel.

## Files likely touched

- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/session-tab-close-action.tsx`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-layout-restore.ts`
- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/lib/env-hidden-sessions.ts`
- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`
- `apps/web/e2e/tests/session/session-tab-close-guard.spec.ts`
- `apps/web/src/locales/en/task.json` and translations

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

- The tab X now closes only its Dockview panel: no confirmation, no session request, and no
  lifecycle change. The sole visible agent panel keeps no X regardless of session count or state.
- The context menu offers **Hide**; **Close Others** closes only sibling agent panels in its
  Dockview group; desktop context-menu Delete keeps its anchored popover and the phone picker keeps
  its confirmation step with unchanged deletion behavior.
- The per-environment hide record persists in session storage beside the env layout. A fresh
  Dockview API rehydrates it after reload, every restore path (fast env switch, saved-layout
  fromJSON, sibling materialization, maximize restore, custom-layout reuse) filters through it, and
  pruning waits for authoritative task-session hydration so an initially empty store cannot erase
  it. Explicit reopen clears the record and restores the same conversation.
- Regressions cover: hidden siblings stay absent through synchronization; reload keeps the hidden
  panel absent until reopened from the + menu; empty-initial-sessions pruning is deferred until
  hydration; the last-panel guard; Close Others scoping; unchanged delete flows.
- Focused suites all green: components/task session-tab tests, lib/state, layout-restore,
  session hooks, `pnpm run typecheck`, `pnpm run i18n:check`, and repo lint.

## Localized-confirmation follow-up

The shipped localization refinement kept this task's contract and moved desktop context-menu
confirmation into an anchored popover and phone confirmation into its Sessions picker step.
Shared warning copy lives in the purpose-neutral
`components/task/session-delete-description.tsx`; the context-menu event and `preventDefault()`
contracts are documented beside their public callback and Radix handler.
