---
id: "04-sidebar-badge-ui"
title: "Render the queued prompt count badge in the task sidebar"
status: done
wave: 3
depends_on: ["02-status-summary-queued-count"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-queued-prompt-count.md"
---

# Task 04: Render the Badge in the Task Sidebar

## Acceptance

- `TaskStatusSummary` (web type) gains `queued_prompt_count?: number`.
  `toKanbanTask` and the WS `task.status_summary.updated` handler already move
  the summary wholesale, so no mapper change is required.
- `buildSidebarItem` maps `summary.queued_prompt_count` to a new
  `queuedCount?: number` on `TaskSwitcherItem`; `TaskRow` forwards it to
  `TaskItem`.
- `TaskItemStatsRow` renders, on the metadata line, an inline-flex badge with
  `IconMail` (from `@tabler/icons-react`) followed by the count whenever
  `queuedCount > 0`. Count 0 (or undefined) renders nothing, and the row's
  early-return condition includes the badge so a row with only a queue badge
  still renders. The badge is `data-testid="sidebar-task-queued-count"`,
  non-interactive, shrink-safe, and must not wrap or overflow the row.
- Accessible name is localized via `useTranslation`:
  `sidebar:queuedPromptCount_one` (`{{count}} queued prompt`) /
  `sidebar:queuedPromptCount_other` (`{{count}} queued prompts`) added to
  `apps/web/src/locales/en/sidebar.json`, with the pseudo-locale regenerated.
  No other literals in the file are migrated (i18n ratchet judges only added
  lines).
- Subtasks and tasks share `TaskItem`, so both get identical treatment.
- Unit tests prove: `buildSidebarItem` carries the count; the badge renders
  for counts > 0 on task and subtask rows, is absent at 0/undefined, carries
  the pluralized accessible name, and does not render when the row would
  otherwise be empty.

## TDD Sequence

1. Write `buildSidebarItem` and `TaskItem` tests for the cases above. Run RED.
2. Implement the type field, item plumbing, row rendering, and i18n keys.
3. Run focused tests GREEN, then typecheck, lint, `i18n:check`, and
   `i18n:ratchet` (see Verification).

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  components/task/task-item.test.tsx \
  components/task/task-session-sidebar-item.test.ts
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web run lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Files Likely Touched

- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/components/task/task-session-sidebar-item.test.ts`
- `apps/web/src/locales/en/sidebar.json`
- generated pseudo-locale catalog

## Dependencies

Task 02 fixes the summary JSON shape (`queued_prompt_count`) this task reads.

## Output Contract

Report RED/GREEN unit evidence, the badge markup, i18n status, typecheck/lint
status, and changed files. Update this task and `plan.md` status in the same
implementation conversation.

## Results

- RED: buildSidebarItem/TaskItem badge tests failed against the missing prop and rendering.
- GREEN: 50 focused unit tests pass (task-item, task-session-sidebar-item, mobile sheet-hooks, task-switcher);
  web typecheck, lint, `i18n:check`, and `i18n:ratchet` all pass.
- The badge renders `IconMail` + count on the metadata line when > 0 (desktop and mobile sheet share
  `TaskItem`), with localized `sidebar:queuedPromptCount_one/_other` aria; count 0 renders nothing.
