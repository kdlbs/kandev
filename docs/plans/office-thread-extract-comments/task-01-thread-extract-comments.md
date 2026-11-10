---
id: "01-thread-extract-comments"
title: "Add Office thread extract comments"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-THREAD-EXTRACTS-001
acceptance_criteria:
  - AC-OFFICE-THREAD-EXTRACTS-001.1
  - AC-OFFICE-THREAD-EXTRACTS-001.2
  - AC-OFFICE-THREAD-EXTRACTS-001.3
  - AC-OFFICE-THREAD-EXTRACTS-001.4
  - AC-OFFICE-THREAD-EXTRACTS-001.5
  - AC-OFFICE-THREAD-EXTRACTS-001.6
system_design:
  - ../../specs/office/system-design/thread-extract-comments.md
---

# Task 01: Add Office thread extract comments

## Summary

Let users attach remarks to selected passages in agent-authored Office task comments. Reuse the shared comment selection surface, task-scoped draft store, existing task comment API, and Office routing lifecycle.

## In scope

- Add desktop and touch extract creation to editable agent comments.
- Preserve pending drafts on request failure and preserve changes made during a pending request.
- Protect source mentions while keeping new user feedback mentions active.
- Add unit, scheduler, and Office E2E coverage.
- Update public Office task guidance and coverage evidence.

## Out of scope

- A new comment endpoint, persisted draft model, or plugin API.
- Cross-tab or cross-device draft synchronization.
- Changes to workflow assignment or wake policy.

## Acceptance

- An editable Office task can create, edit, remove, and submit extracts from agent comments. Read-only tasks cannot create them.
- A successful comment request removes only unchanged submitted extracts. A failed request retains drafts.
- Source mentions do not route again; explicit user mentions continue to route. Desktop and phone Add paths retain the remark when Run is unavailable.

## ASCII UI preview

This work order uses the [full UI-01 and UI-02 previews in the plan](plan.md#ascii-ui-preview). UI-01 maps to `AC-OFFICE-THREAD-EXTRACTS-001.1`, `.2`, `.3`, and `.4`. UI-02 maps to `AC-OFFICE-THREAD-EXTRACTS-001.1`, `.2`, and `.6`.

```text
UI-01 Desktop: Agent comment -> selected text -> remark popover -> Add -> composer chip -> Send
UI-02 Phone:   Agent comment -> selected text -> remark drawer -> Add -> composer chip -> Send
```

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/simple/task-chat.extracts.test.tsx components/task/chat/messages/message-comment-surface.test.tsx hooks/domains/comments/use-thread-extracts.test.ts lib/state/slices/comments/format.test.ts lib/state/slices/comments/message-comments.test.ts)
(cd apps/backend && go test ./internal/office/scheduler)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/office/thread-extract-comments.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-thread-extract-comments.spec.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/chat/messages/message-comment-surface.tsx components/task/simple/comment-extract-surface.tsx components/task/simple/task-chat.tsx hooks/domains/comments/use-thread-extracts.ts lib/state/slices/comments/format.ts lib/state/slices/comments/message-comments.test.ts)
(cd apps/web && pnpm exec prettier --check components/task/chat/messages/message-comment-surface.tsx components/task/simple/comment-extract-surface.tsx components/task/simple/task-chat.tsx hooks/domains/comments/use-thread-extracts.ts lib/state/slices/comments/format.ts lib/state/slices/comments/message-comments.test.ts)
(python3 scripts/list-docs.py validate)
(python3 scripts/lint-spec-files.test.py)
(python3 scripts/lint-spec-files.py --all)
(node --test scripts/validate-public-docs.test.mjs)
(node scripts/validate-public-docs.mjs)
(git diff --check)
```

## Files likely touched

- `apps/backend/internal/office/scheduler/mentions.go`
- `apps/backend/internal/office/scheduler/mentions_test.go`
- `apps/web/components/task/chat/messages/message-comment-surface.tsx`
- `apps/web/components/task/simple/comment-extract-surface.tsx`
- `apps/web/components/task/simple/task-chat.tsx`
- `apps/web/hooks/domains/comments/use-thread-extracts.ts`
- `apps/web/lib/state/slices/comments/format.ts`
- `apps/web/e2e/tests/office/thread-extract-comments.spec.ts`
- `apps/web/e2e/tests/office/mobile-thread-extract-comments.spec.ts`
- `docs/public/office-tasks.md`
- `docs/public/coverage.json`

## Dependencies

None.

## Risks

- Drafts are browser-tab scoped until the user submits the task comment.
- Source mentions must remain inactive without changing mentions in typed feedback.

## Parallelism

`sequential`

## Inputs

- [Office thread extract comments requirements](../../specs/office/requirements/thread-extract-comments.md).
- [Office thread extract comments system design](../../specs/office/system-design/thread-extract-comments.md).
- Existing Office task comments and shared chat comment selection components.

## Results

The feature and targeted regression coverage are implemented on the PR branch. The main-branch conflict is resolved. The focused frontend suite passed with 52 tests, the scheduler test and web typecheck passed, and the desktop, phone, and previously failing background handoff E2E cases passed locally. Changed-file lint, formatting, and documentation validators passed. Hosted CI will verify the pushed branch.
