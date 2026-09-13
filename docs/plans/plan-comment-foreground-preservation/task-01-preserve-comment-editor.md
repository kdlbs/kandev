---
id: "01-preserve-comment-editor"
title: "Preserve the mounted comment editor"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.9
  - AC-TASKS-PLAN-COMMENTS-001.10
  - AC-TASKS-PLAN-COMMENTS-001.11
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 01: Preserve the mounted comment editor

## Summary

Keep loaded Plan content mounted during refresh. Prove new comment text and
unsaved edits survive browser foreground return on desktop and phone.

## In scope

Narrow the panel loading branch, add failing-first component regressions and
real loader/foreground evidence, and add desktop/phone browser regressions.

## Out of scope

Backend/storage changes, refresh suppression, layout changes, recovery after
reload/dismissal/task navigation, and unrelated comments.

## Acceptance

1. Pending, successful, and failed same-plan refresh preserves the same input
   node, text, selected text, and add/edit mode on desktop and phone.
2. Foreground return still requests authoritative state, never saves or sends
   draft text, and explicit Add/Update receives the preserved value.
3. Initial loading, task change, and confirmed plan deletion/replacement never
   expose or act on the outgoing draft. Existing mutation guards still pass.

## ASCII UI preview

Excerpt of [UI-01 and UI-02](plan.md#ascii-ui-preview), AC-001.9 through .11:

```text
Desktop Popover:                Phone bottom Drawer:
  "Selected text"              | Comment                  |
  [Unfinished feedback...]     | "Selected text"          |
                 [Add] [Run]   | [Unfinished feedback...] |
                              |              [Add] [Run] |
```

Refresh leaves these surfaces mounted. Editing retains Update/Delete and
existing inline failure feedback. Keep phone dynamic height, one internal
scroll owner, safe-area clearance, and 44 px actions. No geometry changes.

## Verification

Run from repository root. Install dependencies once in this fresh worktree.
Run new component tests first against unchanged production code and record
expected failure (input removed or body reset), then implement and run:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- components/task/task-plan-panel.refresh.test.tsx components/task/task-plan-panel.session-switch.test.tsx components/task/plan-selection-popover.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/plan-comment-loading.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/task-plan-panel.tsx components/task/task-plan-panel.refresh.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/plan-comment-loading.test.ts e2e/tests/session/task-plan-comments.spec.ts e2e/tests/session/mobile-task-plan-comments.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/task-plan-comments.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the managed E2E runner's build step; never use stale assets. Run desktop
and phone sequentially against test-base isolated data. Use causal transport
waits rather than arbitrary sleeps. Inspect the rendered phone Drawer and
desktop Popover after return. Record any headless limitation and the manual
Alt-Tab result separately. Add no automatic focus stealing.

## Files likely touched

- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/task-plan-panel.refresh.test.tsx` (new)
- `apps/web/hooks/domains/comments/use-plan-comments.test.tsx`
- `apps/web/hooks/domains/comments/plan-comment-loading.test.ts`
- `apps/web/e2e/tests/session/task-plan-comments.spec.ts`
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts`

## Dependencies

None. Current backend and loader behavior supply the refresh path.

## Risks

Keep shared loading semantics and owner isolation. Mocking the Popover/Drawer
would hide the regression; use real inputs and assert mounted identity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-comments.md), AC-001.9 to .11.
- [Design](../../specs/tasks/system-design/plan-comments.md#open-comment-editor-during-background-reads).
- Existing `task-plan-panel.session-switch.test.tsx`,
  `plan-selection-popover.test.tsx`, and plan-comment E2E suites.
- [Historical recovery package](../plan-comment-recovery/plan.md).

## Results

Pending implementation. Diagnosis is a read-only source trace, not executed
component or browser regression evidence.
