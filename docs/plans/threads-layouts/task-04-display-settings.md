---
id: "04-display-settings"
title: "Expose Display settings"
status: done
wave: 4
depends_on:
  - "03-composer-disclosure"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-005
  - REQ-UI-THREADS-DECK-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-005.1
  - AC-UI-THREADS-SAVED-VIEWS-005.2
  - AC-UI-THREADS-SAVED-VIEWS-005.3
  - AC-UI-THREADS-SAVED-VIEWS-005.4
  - AC-UI-THREADS-SAVED-VIEWS-005.5
  - AC-UI-THREADS-SAVED-VIEWS-005.6
  - AC-UI-THREADS-SAVED-VIEWS-005.7
  - AC-UI-THREADS-DECK-004.6
  - AC-UI-THREADS-DECK-004.7
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 04: Expose Display settings

## Summary

This completed order records the initial implementation. Its standalone layout
shortcut is superseded by the user's feedback in
[Task 06](task-06-presentation-polish.md). Current layout selection belongs only
inside the configurator; the Results below remain historical evidence.

Expose layout and auto-hide through the existing saved-view editor, with one
compact desktop layout shortcut. Complete the user flow across saved views,
desktop/phone presentation, persistence, and recovery.

## In scope

- Add Display to `ThreadsViewEditor` using existing Select/Switch controls,
  translated descriptions, and existing save actions.
- Add the fine-pointer layout shortcut to `ThreadsViewControls`; edit the
  same draft and open its editor. Preserve header containment and dirty state.
- Reuse the phone/tablet drawer's editor page; show wider-screen layout and
  visible-touch-composer explanations. Surface the render-only grid-height reason.
- Relabel the limit Maximum chats with unchanged wire semantics and defaults.
  Complete five-language catalogs and generated pseudo/Traditional Chinese.
- Prove UI-driven Save/Save as/Duplicate/Discard, reload, cross-client updates,
  rejected writes/retry, unchanged sorting, and both presentation preferences.

## Out of scope

A second settings owner, new save coordination, task-query changes, automatic
default/limit changes, new phone grids, or public documentation.

## Acceptance

1. Desktop and touch entry points edit the same view draft and expose visible
   descriptions, current values, Save/Discard, dirty state, and recovery.
2. End-to-end view operations retain both preferences while preserving task
   selection, order, limits, session drafts, and another listing's settings.
3. Phone and short-height fallbacks are explained and never write over Grid;
   localized controls remain contained and usable by pointer/keyboard/touch.

## ASCII UI preview

UI-04 excerpt; [full settings and failure preview](plan.md#ui-04-display-settings-and-recovery).

```text
Desktop: [View v] [Settings] [Layout: Grid v] [Listing views]

+---------------------------------------+
| Display                               |
| Layout                   [Grid v]     |
| Two rows for monitoring conversations.|
| Auto-hide composer             [On]   |
| Hover or focus a chat to reply.       |
| Touch keeps the composer visible.     |
| Maximum chats                  [5]    |
| Counts chats across both rows.        |
| [Discard] [Save as]            [Save] |
+---------------------------------------+
```

On phone this section is in the existing inset drawer's editor page:
fixed header/back, one scrolling body, safe-area padding, 44px controls.
The phone itself stays UI-05, one conversation. Short-height helper text
explains the Columns fallback while Grid remains selected. A rejected sync
uses the existing error/retry region. Covers the assigned saved-view criteria.

## Verification

Use /tdd and the existing drawer/control test setup. Generate Traditional
Chinese and pseudo catalogs using the repository scripts before final checks.
The E2E files below cover the integrated behavior.

```bash
(cd apps/web && pnpm exec vitest run components/threads/threads-view-controls.test.tsx components/threads/threads-view-controls-recovery.test.tsx components/threads/threads-view-editor-actions.test.tsx components/threads/threads-view-editor-utils.test.ts lib/state/slices/ui/thread-view-actions.test.ts lib/threads/thread-view-query.test.ts app/threads/threads-page-client.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/threads/threads-view-controls.tsx components/threads/threads-view-editor.tsx components/threads/threads-view-editor-sections.tsx components/threads/threads-view-editor-actions.tsx app/threads/threads-page-client.tsx --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-display-settings.spec.ts tests/task/threads-layouts.spec.ts tests/task/threads-composer-disclosure.spec.ts tests/task/threads-view.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-display-settings.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-threads-task-actions.spec.ts -- --retries=0)
git diff --check
```

The combined browser run is required here because the new controls finally
exercise persistence, composition, and disclosure together. Do not overlap
these managed suites or override their worker/shard budget. Compare UI-01
through UI-05 in the rendered captures, including long translated labels and
a coarse-pointer tablet. Restore settings and second browser contexts on
failure as well as success.

## Files likely touched

- `apps/web/components/threads/threads-view-controls.tsx`,
  `threads-view-editor.tsx`, and `threads-view-editor-sections.tsx`.
- `threads-view-editor-actions.tsx` only for shared draft action integration;
  extract a small Display component if component limits require it.
- `apps/web/app/threads/threads-page-client.tsx` for effective fallback context
  between the existing board/header render slot and controls.
- Existing control/recovery/editor tests named above; their fixtures.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/threads.json`
- New `apps/web/e2e/tests/task/threads-display-settings.spec.ts` and
  `mobile-threads-display-settings.spec.ts`; shared presentation helpers.

## Dependencies

Tasks 01-03. All underlying behavior must be usable before its controls ship.

## Risks

An instant layout shortcut can hide how to save a draft. Opening the existing
editor resolves that without a second save flow. A Grid layout must not
reinterpret Maximum chats as visual columns. Responsive helpers must not
persist the effective fallback or erase unsaved query edits.

## Parallelism

sequential

## Inputs

- [Saved presentation requirements](../../specs/ui/requirements/threads-saved-views.md#req-ui-threads-saved-views-005-saved-presentation-preferences)
- [Presentation design](../../specs/ui/system-design/threads-saved-views.md#presentation-preferences)
- Existing `ThreadsViewControls`, `ThreadsViewEditor`, drawer recovery,
  saved-view actions, and phone view-editor E2E.

## Results

PR #3626 follow-up: the shared saved-view action and desktop/touch lists protect
unresolved drafts until Save or Discard. Native mobile deletion confirmation
from the base branch is preserved. Final commands and results are in the
[review remediation record](plan.md#pr-review-remediation-2026-09-12).
The original implementation evidence below remains historical.

Done, 2026-09-11.

Display uses the existing saved-view draft and save/recovery actions. The
fine-pointer layout shortcut opens that editor after changing the draft;
phones and coarse-pointer tablets use its existing drawer. Grid-height and
phone fallbacks are explained without changing saved preferences. Maximum
chats keeps its existing wire field and total-chat semantics. All five
languages, generated Traditional Chinese, and pseudo copy are complete.

RED/GREEN evidence: four control assertions failed before implementation
(shortcut edit access, draft values, coarse-tablet drawer, and limit wording).
They pass with the shared Display controls. Browser integration found a
3px overflow from the switch's enlarged touch hit area at 320px. Keeping the
hit area within the drawer padding fixed it without shrinking the touch
target. Rejected-write coverage holds both rapid requests, then verifies
rollback and Retry of the latest desired draft across two clients.

Validation passed:

- The seven listed unit targets plus `threads-board.test.tsx`: 102 tests
  across eight files.
- `pnpm run typecheck`; scoped ESLint for the changed page, board, editor,
  controls, new `threads-view-display.tsx`, tests and presentation helpers;
  `pnpm run i18n:check`; `pnpm run i18n:ratchet`; `git diff --check`.
- Locale generation used `pnpm run i18n:zh-hant --namespace threads` and
  `pnpm run i18n:pseudo`. The managed runner rebuilt backend and web assets;
  after the final web-only style fix, `pnpm run build:e2e` refreshed the web
  bundle. The two listed managed browser commands then ran sequentially with
  `--no-build` and `--retries=0`: **23 desktop and 19 mobile tests passed**.
- Integration covers Save/Save as/Duplicate/Discard, reload, shared-client
  updates, failed-write recovery, unchanged ordering and other listing
  settings, grid geometry and activation, CI-only disclosure, and existing
  Threads navigation and task actions. Added touch coverage exercises native
  model selection, cancellation-pending feedback, and attachments.
- Inspected light/dark, 768/820px keyboard/pseudo, short-height fallback,
  write-recovery, 320px phone/pseudo, and touch-tablet captures. No sideways
  document/drawer overflow. Phone checks use browser emulation, not a physical
  device or hardware keyboard.

Task 05 can document the verified behavior. No commit, push, or deployment.
