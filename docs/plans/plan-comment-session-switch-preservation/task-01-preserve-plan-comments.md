---
id: "01-preserve-plan-comments"
title: "Preserve plan comments across session switches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PLAN-COMMENT-DRAFTS-001
acceptance_criteria:
  - AC-UI-PLAN-COMMENT-DRAFTS-001.1
  - AC-UI-PLAN-COMMENT-DRAFTS-001.2
  - AC-UI-PLAN-COMMENT-DRAFTS-001.3
  - AC-UI-PLAN-COMMENT-DRAFTS-001.4
  - AC-UI-PLAN-COMMENT-DRAFTS-001.5
  - AC-UI-PLAN-COMMENT-DRAFTS-001.6
system_design:
  - ../../specs/ui/system-design/plan-comment-drafts.md
---

# Task 01: Preserve Plan Comments Across Session Switches

## Summary

Separate Tiptap projection reconciliation from user-authored mark deletion so
changing Agent sessions cannot remove pending comments from browser state.
Reset stale comment-editor state on session changes and verify the full user
flow with focused tests.

## In scope

- Add private transaction provenance to programmatic comment-mark
  reconciliation.
- Preserve orphan cleanup for real destructive plan edits.
- Reset open plan-comment selection/edit state when `activeSessionId` changes.
- Add unit, component, and desktop multi-session E2E regressions.

## Out of scope

- Comment data-model, storage-key, API, backend, or WebSocket changes.
- Moving comments between sessions.
- Mobile or desktop layout changes.
- Recovering previously deleted comments.

## Acceptance

- Replacing the editor's session comment projection removes old visible marks
  without calling the destructive comment callback or changing stored drafts.
- A real user document edit that removes a marked range still deletes its
  pending comment exactly once, and a session change closes stale edit state.
- Returning to the owning session restores the pending comment's badge and
  exact feedback in the production-built desktop user flow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run components/editors/tiptap/tiptap-plan-editor.test.tsx components/task/task-plan-panel.session-switch.test.tsx
cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts -- --grep "preserves pending plan comments across session switches"
```

After specification or plan edits:

```bash
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/editors/tiptap/comment-mark.tsx`
- `apps/web/components/editors/tiptap/tiptap-plan-editor.test.tsx`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/use-plan-selection.ts`
- `apps/web/components/task/task-plan-panel.session-switch.test.tsx`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `docs/plans/plan-comment-session-switch-preservation/plan.md`
- `docs/plans/plan-comment-session-switch-preservation/task-01-preserve-plan-comments.md`

## Dependencies

None.

## Risks

- Transaction suppression must be limited to tagged projection reconciliation;
  otherwise genuine orphaned comments could remain.
- The session-change reset must not run on ordinary comment-store updates or
  close an editor while the owning session remains selected.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-PLAN-COMMENT-DRAFTS-001` and its system design.
- Current `CommentMark`, `TipTapPlanEditor`, `TaskPlanPanel`, and comment-store
  behavior.
- Existing multi-session and Plan comment Playwright patterns.
- Confirmed temporary Vitest reproduction recorded in `plan.md`.

## Results

- Tagged every comment-mark projection transaction with private provenance and
  limited orphan cleanup to IDs removed by untagged document transactions and
  still absent from final document state.
- Cleared local selection, browser selection, and global comment-edit identity
  when `activeSessionId` changes.
- RED unit evidence: projection replacement invoked the destructive callback;
  session replacement left the old comment textarea open.
- RED browser evidence: the primary-session badge count remained zero after a
  primary to sibling to primary round trip.
- GREEN unit command:
  `cd apps/web && pnpm exec vitest run components/editors/tiptap/tiptap-plan-editor.test.tsx components/task/task-plan-panel.session-switch.test.tsx`
  passed 7 tests in 2 files.
- GREEN browser command:
  `cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts -- --grep "preserves pending plan comments across session switches"`
  passed its Chromium scenario against the production build.
- `cd apps/web && pnpm run typecheck` and focused ESLint passed. No backend,
  API, schema, persistence-key, mobile layout, or touch behavior changed.
