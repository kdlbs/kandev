---
id: "03-visible-queue-removal-ui"
title: "Expose removal for every visible queue row"
status: completed
wave: 3
depends_on: ["01-authoritative-queue-cancellation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-management.md"
---

# Task 03: Expose Removal for Every Visible Queue Row

## Acceptance

- User, agent, workflow, and server rows all render an individual Remove
  action; only user rows render Edit.
- Existing merge eligibility and sender-provenance rendering remain unchanged.
- Clear-all and individual removal reconcile to authoritative queue status on
  success, race, and error; localized feedback replaces stale optimistic state.
- Visible positions compact to `#1` through `#N` after remove, merge, or drain,
  even when durable FIFO position keys retain gaps.
- On coarse-pointer/mobile viewports, queue actions are always visible and
  expose at least 44px hit areas. Desktop keeps compact hover behavior.
- The queue keeps one internal scroll region and never pushes the composer off
  the phone viewport.

## TDD Sequence

1. Add component tests proving `canRemove` is independent from `canEdit` for
   each provenance kind. Add hook tests for clear/remove success and refetch on
   failure. Run RED.
2. Split the row-action props and render Remove outside the edit-only branch.
3. Update clear/remove reconciliation and localize all newly touched queue
   labels, titles, and errors.
4. Add coarse-pointer responsive classes and focused assertions for visible
   44px controls without changing the queue scroll owner.
5. Run focused unit tests, typecheck, lint, and i18n checks GREEN.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  components/task/chat/queued-ghost-list.test.tsx \
  components/task/chat/queued-ghost-message.test.tsx \
  hooks/domains/session/use-queue.test.ts
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Files Likely Touched

- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-list.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`
- `apps/web/src/locales/en/chat.json`
- generated pseudo-locale catalog

## Dependencies

Task 01 supplies the backend behavior this UI exposes.

## Parallelism

Parallel-safe with Task 04 after backend dependencies. They touch separate
components, API domains, and translation namespaces. No subagent is authorized
by this task file.

## Output Contract

Report RED/GREEN unit evidence, provenance matrix, mobile hit-area result,
typecheck/lint/i18n status, changed files, and residual UX risks. Update this
task and `plan.md` status in the same implementation conversation.

## Results

- RED: the focused component/hook suite failed 9 tests because removal was
  coupled to editability, coarse-pointer hit-area classes were absent, and
  clear/remove skipped authoritative reconciliation paths.
- GREEN: 73 focused tests pass across `queued-ghost-list`,
  `queued-ghost-message`, and `use-queue`.
- User, agent, workflow, and server rows expose Remove; only `queued_by=user`
  exposes Edit. Existing merge eligibility is unchanged.
- Clear and Remove refetch authoritative status after success, drain races,
  and failures; non-race failures restore state and surface localized errors.
- Desktop row controls remain 24px hover actions. Coarse-pointer controls are
  always visible with 44px hit areas; the existing queue list remains the one
  panel scroll owner.
- GREEN: web typecheck, lint, `i18n:check`, and `i18n:ratchet`.
- Follow-up RED/GREEN: a sparse-position render failed with `#1, #3`; the
  queue panel now derives display ordinals from rendered order and shows
  `#1, #2` without mutating durable FIFO keys.
