---
id: "03-collection-access"
title: "Expose the independent collection editor"
status: complete
wave: 2
depends_on:
  - "02-draft-preservation"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 03: Expose the independent collection editor

## Summary

Open a task-owned collection editor from the composer and a session-independent
fallback. Reuse the same editor in preview chrome without requiring an iframe
for saved-item operations.

## In scope

- Shared saved-item list, evidence, edit, delete, and revision-guarded clear.
- Task-scoped entry from the composer context chip.
- A visible fallback for stopped-session, launch-error, and no-session states.
- Desktop Popover and touch Drawer with shared mutation behavior.
- Error retention, confirmation, focus return, localization, and focused E2E.

## Out of scope

- A new route, store, backend action, per-session feedback copy, or Send button.
- Starting an agent or requiring preview recovery to edit feedback.
- Capture completion, screenshot rendering, URL sanitization, or other open PR findings.
- Changes to archived-task permissions.

## Root cause and reproduction

`buildContextItems` creates a preview-feedback item without `onOpen`.
`PreviewFeedbackItem` renders a read-only list inside `ContextChip`.
Saved-item mutations live inside a capture surface tied to preview chrome.

Seed pending feedback, close the preview, and open the chip. The current
surface has no edit or delete action. Stop the source session and repeat.
The stored task data remains, but its management UI is unavailable.

`ChatInputContainer` also hides the ordinary input after some launch errors
and replaces it with `SessionStoppedBanner` in stopped-session states.
A fix only inside the chip therefore does not satisfy session independence.

## Implementation approach

1. Add failing component tests for chip activation and a missing source session.
2. Extract saved-item rendering and mutations into `preview-feedback-collection.tsx`.
3. Pass task identity and the existing domain-hook operations into that surface.
4. Keep capture and upload controls outside the saved collection component.
5. Supply a real open callback through `buildContextItems` and the composer state.
6. Mount its controlled surface at `ChatInputArea`, outside hidden or replaced input branches.
7. Expose the same task-scoped trigger when the ordinary context chip is unavailable.
8. Use `taskId` from panel state even when `resolvedSessionId` is null.
9. Reuse the extracted list in `PreviewFeedbackControls`.

Use the existing shared Popover primitive for desktop. Use `useTouchDrawer`
and the task `MobilePickerSheet` composition for coarse pointers. A click or
keyboard activation opens the editor directly. Do not add an intermediate
read-only Drawer that requires a second Open action.

Preserve exact item versions and collection revisions. Await edit success
before closing the editor. On conflict, retain the typed edit and expose the
current snapshot. Disable duplicate mutations until settlement. Clear requires
confirmation and the displayed revision. Reuse the existing confirmation host
on touch rather than stacking another Drawer.

Retain captured text and element evidence as escaped content. This extraction
does not claim to resolve the separately reported persisted-image preview bug.
Do not reduce the current evidence display or overwrite immutable captures.

## Acceptance

1. An active task exposes the same collection with the preview closed, source
   session stopped, or no selected session. Opening it sends no prompt and
   performs no agent start or recovery.
2. Edit, delete, and confirmed clear update the task collection. Failed edits
   retain input, stale mutations preserve newer rows, and reload shows saved state.
3. Desktop and touch provide equivalent operations. Focus returns correctly,
   touch targets meet 44px, and the Drawer has one contained scroll region.

## ASCII UI preview

Use [UI-R2 in the plan](plan.md#ui-r2-task-collection-from-the-composer).
The shared view supports normal, empty, load-error, and edit-conflict states.

```text
Desktop chip -> +------------------------------------+
                | Pending feedback (2)       [Close] |
                | /checkout - captured evidence      |
                | Comment             [Edit][Delete] |
                | [Edit field]        [Cancel][Save] |
                |                        [Clear all] |
                +------------------------------------+

Phone entry ->  +------------------------------------+
bottom Drawer   | Pending feedback (2)       [Close] | fixed
                | /checkout                          |
                | Evidence and comment               | scroll
                | [Edit]                    [Delete] |
                | /account ...                       |
                | [Clear all]                        |
                +------------------------------------+
                  safe-area clearance

No session: [Preview feedback (2)] [existing recovery UI]
Conflict:   [Typed edit stays] [Current version available]
Empty:      [No pending feedback]                 [Close]
```

The collection is a temporary review task, so a Drawer fits the phone flow.
Do not create a new navigation destination. Use `mobile-picker-sheet.tsx` as
the fixed-header and scroll-body reference. Keep actions visible without hover.
Use dynamic viewport bounds and the existing coarse-pointer input font size.
All new copy needs five locale translations and the generated pseudo locale.

## Tests

- New `preview-feedback-collection.test.tsx`: operations, exact versions,
  conflict preservation, empty/error states, desktop focus, and touch Drawer.
- `chat-context-items.test.ts`: open callback and unchanged task-wide count.
- `chat-input-area.test.tsx`: no session, stopped session, launch error, and
  ordinary composer. Assert the collection remains reachable in each case.
- `preview-feedback-controls.test.tsx`: both preview adapters reuse the list
  without requiring capture mode for management.
- Retain existing Browser and HTML-preview host tests.
- Extend the desktop and mobile E2E files named in the plan. Add reload after
  edit, delete from a stopped-session state, and a no-session component case.
- Include Task 02's create-failure draft scenario in both browser projects.
  Use the existing test fixture and public UI entry points. Do not require
  live selection merely to seed a collection-management test.

## Verification

Run from the repository root. For a fresh worktree, first install dependencies
with `(cd apps && pnpm install --frozen-lockfile)`. The collection test file is
new and must exist before these commands run.

```bash
(cd apps/web && pnpm exec vitest run components/task/inspector/preview-feedback-collection.test.tsx components/task/inspector/preview-feedback-controls.test.tsx components/task/chat-context-items.test.ts components/task/chat/chat-input-area.test.tsx components/task/chat/context-items/context-chip.test.tsx components/task/browser-panel.test.tsx components/task/html-preview-content.test.tsx hooks/use-preview-capture.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/inspector components/task/chat-context-items.ts components/task/chat/chat-input-area.tsx components/task/chat/use-chat-panel-state.ts components/task/chat/context-items/preview-feedback-item.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/preview/preview-feedback.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-html-preview.spec.ts)
git diff --check
```

Run the managed E2E commands sequentially. They build the required artifacts.
Do not use worker overrides, headed mode, or stale builds. Record actual
scenario counts and the rendered phone comparison with UI-R2. Add any new
production files to the scoped ESLint command before completion.

## Files likely touched

- `apps/web/components/task/inspector/preview-feedback-collection.tsx` (new)
- `apps/web/components/task/inspector/preview-feedback-collection.test.tsx` (new)
- `apps/web/components/task/inspector/preview-feedback-controls.tsx`
- `apps/web/components/task/inspector/preview-feedback-controls.test.tsx`
- `apps/web/components/task/chat-context-items.ts`
- `apps/web/components/task/chat-context-items.test.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/chat/chat-input-area.test.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/chat/context-items/preview-feedback-item.tsx`
- `apps/web/lib/types/context.ts`
- `apps/web/e2e/tests/preview/preview-feedback.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`

## Dependencies

Task 02 establishes editor state ownership before extraction.
Run after Task 01 in the package sequence. Queue changes are not a dependency
of the collection UI, but existing Send scenarios remain regression coverage.

## Risks

- A trigger nested inside a hidden composer does not provide independent access.
- Multiple mounted composers need shared data, not shared global open state.
- Interactive forms in a HoverCard lose reliable keyboard and touch behavior.
- An old item version must not silently replace a newer comment.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md) and its mobile design contract.
- [Design: Collection access without capture](../../specs/tasks/system-design/web-preview-feedback.md#collection-access-without-capture).
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx`.
- `apps/web/components/confirmation/AGENTS.md` before confirmation changes.
- Existing `usePreviewFeedback`, preview-control, context-chip, and HTML E2E tests.

## Results

Implemented. Saved feedback management is now a shared task-scoped collection
surface. The composer chip opens it when a session is available, and a
session-independent trigger remains visible for completed, stopped, launch-error,
and no-session states. Preview capture controls reuse the same editor without
requiring an iframe for edit, delete, or clear operations.

Desktop uses a controlled Popover. Touch uses the existing Drawer picker
composition with a fixed header, one scroll body, safe-area padding, 44-pixel
targets, focus return, localized errors, and revision-guarded mutations.

Component coverage passed for collection operations, conflict retention, task
chip wiring, fallback predicates, and preview integration. Desktop Chromium
preview feedback E2E passed, and mobile Chrome HTML preview E2E passed all 3
tests, including collection management after terminal session hydration and
reload restoration. Full web lint and localization gates passed.
