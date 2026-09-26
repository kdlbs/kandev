---
id: "05-e2e-and-documentation"
title: "Complete feature verification"
status: completed
wave: 5
depends_on:
  - "01-atomic-move-admission"
  - "02-deferred-destination-lifecycle"
  - "03-kanban-queue-sections"
  - "04-sidebar-queue-position"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 05: Complete Feature Verification

## Acceptance

- Desktop E2E uses the UI to move a task into a full step, verifies successful
  destination queue placement, admitted/queued Kanban areas, WIP count, and
  sidebar queue-position tooltip, then frees capacity and verifies promotion.
- Mobile Chrome E2E repeats the move through the focused-column/task-switcher
  flow, verifies the queue icon tooltip, promotion, touch usability, and no
  page-level horizontal overflow.
- Desktop and mobile workflow-settings coverage opens the `Pull from` info
  tooltip. The guidance identifies automatic feeder intake as optional and
  distinguishes it from direct or automatic transitions.
- Tests seed prerequisite workflow/task state through the API but perform and
  assert the behavior under test through visible UI interactions.
- Existing creation-overflow and feeder auto-pull E2E coverage stays green.
- When PR asset capture is available, screenshots are reviewed at desktop and
  mobile widths for section hierarchy, truncation, chip legibility, and
  queue-position consistency. Capture is best effort and environment failures
  are recorded without blocking functional verification.
- Public task/workflow documentation states that full-step moves queue in the
  destination, destination queues promote before feeders, and the UI exposes
  queued cards and position.
- Focused backend/frontend suites and documentation validation pass; broader
  regressions are reported without hiding environment-only failures.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/kanban/wip-overflow-queue.spec.ts
pnpm e2e:run --project mobile-chrome \
  tests/kanban/mobile-wip-overflow-queue.spec.ts
pnpm e2e:run tests/workflow/workflow-settings.spec.ts
pnpm e2e:run --project mobile-chrome \
  tests/workflow/mobile-workflow-settings.spec.ts

cd ../../
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs

make -C apps/backend test
cd apps
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Implementation Result

- Desktop queue E2E passed: `wip-overflow-queue.spec.ts`, 2 tests.
- Mobile queue E2E passed: `mobile-wip-overflow-queue.spec.ts`, 2 tests.
- Desktop and mobile workflow-settings guidance checks each passed 1 test.
  They cover both the no-feeder and selected-feeder messages.
- The managed E2E checks assert admitted counts, queued sections, sidebar icon
  tooltip position, promotion, touch interaction, and no horizontal
  overflow. Existing queue overflow behavior remains green.
- The backend boot-state serializer regression test verifies that WIP limits,
  feeder configuration, admission, and queue metadata are present in the
  first-paint Kanban payload.
- After the review hardening, the desktop and mobile queue suites were rerun
  against the updated selectors and each passed two tests.
- The pseudo-locale E2E build passed. The focused queue helper and sidebar
  Vitest suites passed 14 tests, and web typecheck, lint, i18n check, and i18n
  ratchet all passed.
- Public documentation validation passed: 58 Node tests and 41 published
  pages. The two public workflow pages now describe destination admission,
  destination-before-feeder promotion, and Kanban/sidebar indicators.
- The broad backend gate passed on its second run. The first run had one
  existing process-manager stderr-sanitization test miss under full-suite
  load; the test passed five isolated runs and the complete process package
  passed 620 tests before the full suite was rerun successfully.
- A separate `CAPTURE_PR_ASSETS=1` screenshot pass was attempted for direct
  visual inspection, but the runner filesystem was at 100% and Docker could
  not allocate overlay space. Functional E2E assertions and captured failure
  diagnostics were available; no screenshot artifact was retained in the
  worktree.

Final verification highlights:

```text
rtk make -C apps/backend test
exit 0

rtk pnpm --filter @kandev/web run build:e2e
exit 0

rtk pnpm --filter @kandev/web exec vitest run \
  lib/kanban/wip-queue.test.ts \
  hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts
2 files, 14 tests passed

rtk pnpm e2e:run tests/kanban/wip-overflow-queue.spec.ts
2 passed

rtk pnpm e2e:run --project mobile-chrome tests/kanban/mobile-wip-overflow-queue.spec.ts
2 passed

rtk node --test scripts/validate-public-docs.test.mjs
58 passed
```

## Files Likely Touched

- `apps/web/e2e/tests/kanban/wip-overflow-queue.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-wip-overflow-queue.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `apps/web/e2e/pages/kanban-page.ts`
- `apps/web/e2e/pages/mobile-kanban-page.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`
- `docs/plans/queued-task-moves/plan.md`
- completed task files in this plan

## Dependencies

Tasks 01 through 04 provide the end-to-end behavior and presentation.

## Parallelism

`sequential`

## Output Contract

Record E2E results, screenshots inspected, public-doc validation, focused and
broad gate results, any environment-only failures, and final changed files.
Mark this task and `plan.md` complete only when the behavior and documentation
are both verified.
