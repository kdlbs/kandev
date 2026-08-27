---
id: "05-e2e-reorder"
title: "E2E reorder desktop and mobile"
status: done
wave: 5
depends_on: ["04-frontend-dnd-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-reorder.md"
---

# Task 05: E2E reorder desktop and mobile

## Acceptance

1. Desktop spec: queue three messages while the agent is busy, open the
   panel, assert `#1/#2/#3`, drag the last row's handle onto the first,
   assert the labels swap and the order survives a reload (server-persisted).
2. Mobile spec (`mobile-*.spec.ts` so the `mobile-chrome` project picks it
   up): the handle is visible without hover, touch-sized, and a touch drag
   reorders with the same persisted result.
3. Desktop also covers: handle not visible before hover; keyboard reorder
   (focus handle, Space, Arrow Up, Space) moves the row.

## Verification

```bash
cd apps/backend && make build-backend   # E2E runs against the production build
cd apps && make build-web              # or use `make test-e2e` which rebuilds both
cd apps/web && pnpm e2e message-queue-reorder
cd apps/web && pnpm e2e mobile-message-queue-reorder
```

Run via the repo E2E flow (`apps/web/e2e/README.md`); `make test-e2e` rebuilds
backend + web and runs the suite.

## Files likely touched

- `apps/web/e2e/tests/chat/message-queue-reorder.spec.ts` — new.
- `apps/web/e2e/tests/chat/mobile-message-queue-reorder.spec.ts` — new.
- Helpers reused: `apps/web/e2e/fixtures/test-base.ts`, `typeWhileBusy`, the
  quick-chat queue setup pattern from `apps/web/e2e/tests/chat/message-queue.spec.ts`.

## Dependencies

Tasks 01–04 (backend action + frontend UI must be present in the build).

## Parallelism

Sequential.

## Inputs

- Spec: `## Scenarios` (persistence across reload, hover reveal, mobile,
  keyboard) and `## Responsive and Mobile Behavior`.
- Plan: `## E2E Tests`.
- Existing patterns: `message-queue.spec.ts` (queueing while busy, panel
  open), `azure-devops.spec.ts` (`locator.dragTo`), `file-tree-drag-drop.spec.ts`
  (manual event dispatch fallback for flaky HTML5 DnD — apply the same
  fallback idea via pointerdown/move/up if `dragTo` proves flaky against
  dnd-kit's PointerSensor).

## Output contract

Summary, files changed, exact commands and pass/fail outcomes, blockers,
risks; update task + plan statuses in the same conversation.

## Results

- Built `make build-backend`, `make build-web`, `make build-e2e-plugin-package` (all required by `global-setup.ts`).
- `cd apps/web && pnpm e2e tests/chat/message-queue-reorder.spec.ts --repeat-each=3` → 6 passed.
- `cd apps/web && pnpm e2e --project=mobile-chrome tests/chat/mobile-message-queue-reorder.spec.ts --repeat-each=3` → 3 passed.
- Specs use stepped mouse moves / delayed touch-pointer events because a single move or key press only activates dnd-kit's PointerSensor without dispatching DragMove (drop resolves no target). Order assertions require the expected rows to occupy the exact consecutive positions starting at the head after explicitly removing the `/slow 30s` keep-busy row, and pin that row to the head when present — a foreign row interleaved between the expected contents fails the assertion (the `/slow 30s` command can remain queued at the head on the mobile flow).
