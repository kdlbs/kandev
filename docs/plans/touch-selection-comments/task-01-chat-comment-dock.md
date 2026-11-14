---
id: "01-chat-comment-dock"
title: "Dock chat and Office Comment actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SELECTION-COMMENT-ACTIONS-001
  - REQ-UI-AGENT-MESSAGE-COMMENTS-001
acceptance_criteria:
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.1
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.2
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.3
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.4
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.5
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.6
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.7
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.8
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.1
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.2
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.3
  - AC-UI-AGENT-MESSAGE-COMMENTS-001.4
system_design:
  - ../../specs/ui/system-design/selection-comment-actions.md
  - ../../specs/ui/system-design/agent-message-selection.md
---

# Task 01: Dock chat and Office Comment actions

## Summary

Deliver the shared owner-bounded dock through Task Chat, Quick Chat, and Office
extracts. Preserve existing comment editors, quotes, eligibility, and delivery.

## In scope

- Recreate the diagnostic as a permanent failing test named
  `keeps the touch Comment action independent of the selection rectangle` in
  `message-comment-surface.test.tsx`. Change only selection geometry and assert
  unchanged dock placement relative to a fixed owner. Retain a desktop control.
- Add the shared provider, dock, and pure positioning helper with geometry,
  modal-origin, composer-boundary, pointer-mode, and cleanup tests.
- Bind the provider to `NativeMessageList`'s real scroll viewport and Office's
  `TaskChat` viewport plus composer boundary. Reuse existing geometry refs.
- Add scoped native selection tracking for touch range changes. Preserve
  owned mouse capture, outside dismissal, open-editor isolation, and identity cleanup.
- Render a labelled dock action on phones and all coarse-pointer viewports.
  Preserve the wider fine-pointer trigger and current drawer/popover editor.
- Keep 44px targets, owner scroll clearance, safe areas, and modal interaction.
- Extend existing chat and Office mobile/desktop tests with geometry and actual
  comment outcomes. Capture the phone composition and record device availability.
- Update **Comment on an agent reply** in `docs/public/developer-tools.md`.
  Reuse existing translation keys unless a new label is necessary. Any new key
  needs English and all six translated catalogs.

## Out of scope

Markdown and code-editor migrations, Plan changes, new comment sources, backend
changes, new plugin props, and native menu suppression.

## Acceptance

- The shared geometry and chat regression fail for the current touch anchoring,
  then pass for Task Chat, Quick Chat, and Office with one owner-scoped action.
- Touch-only selection changes retain the current quote through docking,
  keyboard movement, and editor activation. Existing eligibility, no-Run Office
  behavior, and unrelated-click dismissal remain intact.
- Mobile Add/Run outcomes, modal hit testing, 44px targets, owner/composer
  containment, content clearance, and desktop floating behavior pass their focused checks.

## ASCII UI preview

UI-01 and UI-02 excerpt from the [full plan](plan.md#ascii-ui-preview):

```text
UI-01: Phone/coarse chat       UI-01: Wider fine-pointer chat
+-------------------------+  +-------------------------+
| Reply / Office extract  |  | [selection] [Comment]   |
| [selection]             |  |                         |
| Native selection menu   |  +-------------------------+
|-------------------------|
| [ Comment ]             |  Dock within history viewport
+-------------------------+
| Existing composer       |  Clear of the dock
| Navigation / safe area   |
+-------------------------+

UI-02: Tap Comment -> existing drawer/popover with exact quote.
Office keeps Add without Run. No dock while the editor is open.
```

Structure and clearances are required. Spacing is illustrative. Criteria
`.1` through `.8` map to the focused chat and Office browser scenarios.

## Verification

Use TDD in this primary session. Before package commands in a fresh worktree,
install missing dependencies with `(cd apps && pnpm install --frozen-lockfile)`.
Dependencies were already present during investigation.

From the repository root, after the named new tests exist:

```bash
(cd apps/web && pnpm exec vitest run \
  components/shared/selection-comment-dock-position.test.ts \
  components/shared/selection-comment-dock.test.tsx \
  components/task/chat/messages/message-comment-surface.test.tsx \
  components/task/chat/message-list-native.test.tsx \
  components/task/simple/chat-activity-tabs.test.tsx \
  components/editors/tiptap/plan-bubble-menu.test.tsx \
  hooks/use-visual-viewport-offset.test.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-agent-message-comments.spec.ts \
  tests/office/mobile-thread-extract-comments.spec.ts \
  tests/task/mobile-plan-formatting-toolbar.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/chat/agent-message-comments.spec.ts \
  tests/office/thread-extract-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/shared/selection-comment-dock.tsx \
  components/shared/selection-comment-dock-position.ts \
  components/shared/selection-comment-dock.test.tsx \
  components/shared/selection-comment-dock-position.test.ts \
  components/task/chat/messages/message-comment-controls.tsx \
  components/task/chat/messages/message-comment-surface.tsx \
  components/task/chat/messages/message-comment-surface.test.tsx \
  components/task/chat/messages/use-message-comment-dom.ts \
  components/task/chat/message-list-native.tsx \
  components/task/chat/message-list-native.test.tsx \
  components/task/simple/task-chat.tsx \
  components/task/simple/chat-activity-tabs.test.tsx \
  e2e/tests/chat/mobile-agent-message-comments.spec.ts \
  e2e/tests/chat/agent-message-comments.spec.ts \
  e2e/tests/chat/agent-message-comments-helpers.ts \
  e2e/tests/office/mobile-thread-extract-comments.spec.ts \
  e2e/tests/office/thread-extract-comments.spec.ts)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the plan's documentation validation and offline coverage preflight before
recording completion. Record each command's actual result. The E2E runner builds
fresh assets. Run commands sequentially and retain process handles until completion.

## Files likely touched

- `apps/web/components/shared/selection-comment-dock.tsx` (new)
- `apps/web/components/shared/selection-comment-dock-position.ts` (new)
- `apps/web/components/shared/selection-comment-dock.test.tsx` (new)
- `apps/web/components/shared/selection-comment-dock-position.test.ts` (new)
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/messages/message-comment-controls.tsx`
- `apps/web/components/task/chat/messages/message-comment-surface.tsx`
- `apps/web/components/task/chat/messages/message-comment-surface.test.tsx`
- `apps/web/components/task/chat/messages/use-message-comment-dom.ts`
- `apps/web/components/task/simple/task-chat.tsx`
- `apps/web/components/task/simple/chat-activity-tabs.test.tsx`
- `apps/web/e2e/tests/chat/mobile-agent-message-comments.spec.ts`
- `apps/web/e2e/tests/chat/agent-message-comments.spec.ts`
- `apps/web/e2e/tests/chat/agent-message-comments-helpers.ts`
- `apps/web/e2e/tests/office/mobile-thread-extract-comments.spec.ts`
- `apps/web/e2e/tests/office/thread-extract-comments.spec.ts`
- `docs/public/developer-tools.md`
- This work order, plan, and referenced specifications for status or design reconciliation.

Existing Plan and visual-viewport tests are compatibility checks. Their production
files need no changes unless an actual shared-helper correction requires one.

## Dependencies

None.

## Risks

- A body portal can escape Quick Chat's modal interaction boundary.
- Office's content and composer share a scroll parent. Use the separate lower
  boundary without splitting the page into competing scroll regions.
- Unscoped selection listeners can restore dismissed or foreign-message actions.
- The shared provider must not measure every mounted message on each render.

## Parallelism

`sequential`

## Inputs

- [Selection comment requirements](../../specs/ui/requirements/selection-comment-actions.md).
- [Shared design](../../specs/ui/system-design/selection-comment-actions.md).
- [Agent-message requirements](../../specs/ui/requirements/agent-message-comments.md)
  and [selection design](../../specs/ui/system-design/agent-message-selection.md).
- Existing Plan dock and `useVisualViewportOffset` implementation and tests.
- Existing `mobile-agent-message-comments.spec.ts` and Office extract fixtures.
- Plan diagnosis and UI-01/UI-02 previews.

## Results

Done. Shared docking and chat/Office touch capture are implemented. The focused
mobile E2E run passed both Task Chat/Quick Chat cases and both Office cases; the
desktop run passed all five agent-message comment cases and the Office extract
case. The shared unit suite and repository validators passed. Public guidance
is updated in `docs/public/developer-tools.md`.

Review remediation: the dock now reserves the viewport-relative owner/composer
clearance. Office applies that space inside the transcript before ChatInput. The
focused mobile Office case passed with the composer visible and the final
transcript paragraph above the dock before and after a synthetic keyboard
viewport change.
