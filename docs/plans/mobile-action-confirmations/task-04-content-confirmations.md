---
id: "04-content-confirmations"
title: "Phone task content confirmations"
status: done
wave: 2
depends_on:
  - "01-shared-mobile-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.1
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.3
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-001.7
  - AC-UI-MOBILE-CONFIRMATION-002.1
  - AC-UI-MOBILE-CONFIRMATION-002.2
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.5
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 04: Phone Task Content Confirmations

## Summary

Move the existing inline file, context-reset, revision-restore, detach, and
walkthrough decisions to the shared phone presentation. Keep target selection
and each action's existing semantics with its current owner.

## In scope

- Route phone inline consumers through standalone or existing-surface hosts;
  preserve normal file/revision rows and stable ownership across menu close.
- Supply explicit localized action/target identity and the existing consequence
  content, including destructive and session/context-specific warnings.
- Add adapter and real mobile-flow coverage for Cancel, confirmation, stale
  target/context removal, and current error/retry behavior.

## Out of scope

File deletion semantics, reset quiescence, revision persistence, task detach
policy, walkthrough data, general review layout, and existing full alerts.

## Acceptance

1. Each listed phone action uses one focused surface, preserves origin state
   on Cancel, and dispatches only the captured eligible target on confirmation.
2. Menu close and host changes neither unmount the decision prematurely nor
   leave hidden confirm controls active; rows retain their geometry.
3. Existing operation-specific effects, errors, and non-phone interactions
   pass focused regressions without introducing shared transport behavior.

## Verification

Read `components/task/chat/AGENTS.md` and `components/review/AGENTS.md` before
editing. Use `/tdd` and `/e2e`; add new revision adapter tests and the mobile
content spec below for detach/walkthrough coverage missing from existing specs.

```bash
cd apps/web
pnpm exec vitest run components/task/file-context-menu.test.tsx components/task/chat/reset-context-button.test.tsx components/task/task-plan-revision-restore-actions.test.tsx components/task/task-detach-confirm-dialog.test.tsx components/review/walkthrough-overlay.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-file-confirmations.spec.ts tests/chat/mobile-reset-context-confirmation.spec.ts tests/task/mobile-plan-restore.spec.ts tests/task/mobile-content-confirmations.spec.ts
pnpm e2e:run --project chromium tests/chat/reset-context-confirmation.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Use the existing isolated test fixtures and active-panel selectors. Capture
one long-name standalone file confirmation and one hosted content confirmation.
Retain full file/folder deletion alerts where they already exist.

## Files likely touched

- `apps/web/components/task/{file-browser-parts,file-context-menu,task-plan-revision-restore-actions,task-detach-confirm-dialog}.tsx` and relevant tests/owning surfaces.
- `apps/web/components/task/chat/reset-context-button.tsx` and `apps/web/components/review/walkthrough-overlay.tsx` with their tests.
- Listed mobile E2E files, the new revision unit test, and affected locale catalogs.

## Dependencies

01 supplies the shared presentation and host.

## Risks

File controls currently change row classes while inline confirmation exists.
Remove the phone-only layout effect at the adapter, without changing tablet
behavior. Review/plan origins may already be modal and require an explicit host.

## Parallelism

`sequential`

## Inputs

Mobile design routing, other consumers, and failure sections; current file,
reset, revision, detach, and walkthrough components/tests and scoped guidance.

## Results

Implemented file deletion, context reset, history-row restore, inline detach,
and walkthrough discard through the shared phone surface. Menu handoffs leave
their source controls mounted; file-session changes unmount row-owned requests.
Quick Chat opts into its existing dialog host, preserving an unsent draft.
Existing full detach and revision-preview alerts remain unchanged.

Verification: 101 focused component cases passed, followed by the additional
phone-tooltip regression and focused refactor checks (25 cases). Typecheck,
changed-file lint, i18n checks, and the ratchet passed. Seven distinct mobile
flows passed, including the existing full-detach regression. The final reset,
Quick Chat, and long-file polish rerun passed all three. Desktop reset passed.
Long-file footer bounds/hit targets and Quick Chat's single-modal/draft behavior
were checked in the real browser; settled screenshots were inspected. The
desktop Quick Chat viewport regression also passed on an isolated rerun;
no timeout or production change was needed for that check.

Reused `mobile-subtask-detachment.spec.ts` and
`review/mobile-walkthrough.spec.ts` for their actual mutation outcomes rather
than duplicating those fixtures. The new `mobile-content-confirmations.spec.ts`
covers the previously untested hosted Quick Chat reset path.
