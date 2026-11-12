---
id: "02-markdown-comment-dock"
title: "Dock Markdown preview Comment actions"
status: done
wave: 2
depends_on:
  - "01-chat-comment-dock"
plan: "plan.md"
requirements:
  - REQ-UI-SELECTION-COMMENT-ACTIONS-001
acceptance_criteria:
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.1
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.2
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.3
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.4
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.5
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.6
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.7
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.8
system_design:
  - ../../specs/ui/system-design/selection-comment-actions.md
---

# Task 02: Dock Markdown preview Comment actions

## Summary

Use the shared dock for Markdown file selections on phones and coarse-pointer
devices. Preserve source-line mapping, the current comment editor, and desktop placement.

## In scope

- Add a failing component regression in the new
  `markdown-preview-content.comments.test.tsx`: moving an eligible selection
  changes its quote while the touch action stays at the owner bottom.
- Bind `MarkdownPreviewCommentOverlays` to the existing preview `scrollRef`
  through the shared host. Keep its desktop trigger behavior.
- Extend `useMarkdownPreviewComments` touch capture for delayed native range
  changes. Preserve ignored targets and prevent stale selection restoration.
- Separate touch geometry updates from the current unconditional resize
  dismissal. Keep a valid quote during keyboard resize and viewport scroll.
- Preserve source line ranges, repository identity, comment submission, and Run.
- Add mobile and desktop browser scenarios using the real Files preview.
  Assert successful pending comment creation, exact quote and source lines,
  touch targets, content clearance, and containment. Capture the phone result.

## Out of scope

Review's separate rendered diff preview, Browser annotations, Markdown parsing
changes, code-editor migration, Plan changes, and a new comment editor.

## Acceptance

- Component and browser regressions fail for selection-based touch placement,
  then pass for owner-bounded docking with the current desktop trigger intact.
- Native touch range changes update the eventual quote and source lines.
  Keyboard movement repositions the action without losing the valid selection.
- Tapping the 44px action opens the current editor once. Add creates the
  expected pending file comment without modifying file content or repository identity.

## ASCII UI preview

UI-01 and UI-02 excerpt from the [full plan](plan.md#ascii-ui-preview):

```text
UI-01: Phone/coarse preview    UI-01: Wider fine-pointer preview
+--------------------------+  +--------------------------+
| File preview toolbar     |  | File preview toolbar     |
|--------------------------|  | [selection] [Comment]    |
| Rendered Markdown        |  |                          |
| [selection]              |  +--------------------------+
| Native selection menu    |
|--------------------------|
| [ Comment ]              |  Dock inside preview viewport
+--------------------------+
| Navigation / safe area   |
+--------------------------+

UI-02: Tap Comment -> current file-comment popover, exact quote and lines.
No dock while the editor is open.
```

The preview keeps one content scroll owner and reserves dock clearance.
Keyboard movement changes its visible lower edge. Structure is required,
while spacing is illustrative. Criteria `.1` through `.8` apply.

## Verification

Use TDD after Task 01. From the repository root, after new tests exist:

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/markdown-preview-content.comments.test.tsx \
  hooks/domains/comments/use-markdown-preview-comments.test.ts \
  components/task/markdown-preview-content.math.test.tsx \
  components/task/markdown-preview-content.external-link.test.tsx \
  components/shared/selection-comment-dock.test.tsx)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-markdown-selection-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/task/markdown-selection-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/task/markdown-preview-content.tsx \
  components/task/markdown-preview-content.comments.test.tsx \
  hooks/domains/comments/use-markdown-preview-comments.ts \
  hooks/domains/comments/use-markdown-preview-comments.test.ts \
  e2e/tests/task/mobile-markdown-selection-comments.spec.ts \
  e2e/tests/task/markdown-selection-comments.spec.ts)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Run the plan's documentation validation and offline coverage preflight before
completion. The managed runner builds fresh assets. Any shared helper change
also requires the helper's tests and Task 01's affected chat browser scenario.
Record exact results and physical-device availability without claiming OS-menu proof.

## Files likely touched

- `apps/web/components/task/markdown-preview-content.tsx`
- `apps/web/components/task/markdown-preview-content.comments.test.tsx` (new)
- `apps/web/hooks/domains/comments/use-markdown-preview-comments.ts`
- `apps/web/hooks/domains/comments/use-markdown-preview-comments.test.ts`
- `apps/web/e2e/tests/task/mobile-markdown-selection-comments.spec.ts` (new)
- `apps/web/e2e/tests/task/markdown-selection-comments.spec.ts` (new)
- A sibling shared E2E helper, if both new specs need the same fixture code.
- This work order and plan for status and result updates.

Reuse Task 01's shared dock API. A necessary shared correction must update
its design and focused tests rather than adding a Markdown-only geometry copy.

## Dependencies

Task 01 supplies the shared host, dock, and positioning helper.

## Risks

- The current resize listener clears overlays. Keeping it on the touch path
  defeats keyboard tracking even when the shared dock geometry is correct.
- A rendered quote can span Markdown source blocks. Preserve the existing
  source-range resolver and require mixed-block regression coverage.
- Portaled actions must remain inside any enclosing modal's interaction scope.

## Parallelism

`sequential`

## Inputs

- [Selection comment requirements](../../specs/ui/requirements/selection-comment-actions.md).
- [Shared design](../../specs/ui/system-design/selection-comment-actions.md),
  especially Geometry, Selection lifecycle, and Activation.
- Existing `use-markdown-preview-comments.test.ts` and source-line mapping helpers.
- Existing preview rendering tests and `e2e/tests/chat/markdown-preview.spec.ts`.
- Task 01's shared dock and UI-01/UI-02 in the plan.

## Results

Done. Markdown preview touch selection uses the shared dock and retains source
line mapping while the visible viewport changes. Its mobile and desktop E2E
cases passed, along with the focused Markdown selection and source-line unit
tests in the 14-file, 71-test suite.

Review remediation: delayed selection capture is generation-fenced and canceled
when ownership ends, an unrelated outside action dismisses the dock, the
comment editor opens or closes, or the preview identity changes. Fake-timer
regressions cover outside dismissal, editor handoff, and identity changes. The
focused mobile preview case passed with an unchanged layout owner while the
visual viewport shrank and panned.
