---
id: "06-kanban-dependency-ui"
title: "Kanban dependency UI"
status: done
wave: 6
depends_on: ["01-core-dependency-relationship"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 06: Kanban Dependency UI

## Acceptance

- The Kanban task type, boot-payload mapper, and WS handlers carry `blocked`,
  `blockedReason`, `dependsOn`, `blocks`, and `startWhenUnblocked`, surviving
  both boot hydration and incremental `task.updated` events.

### Dependency chip above the composer

- A dependency chip renders in the chat status row directly above the composer,
  alongside `PRStatusChip`, in `components/task/chat/chat-input-area.tsx`
  (`data-testid="chat-status-bar"`).
- The same chip renders in `components/task/passthrough-toolbar.tsx`
  (`data-testid="passthrough-status-row"`), which mounts its own copy of
  `PRStatusChip`. Both rows must show it or the chip disappears in passthrough
  layouts.
- `shouldRenderChatStatusBar(...)` is extended with the chip's presence.
  Without that, a task whose only status content is the dependency chip renders
  no row at all — the predicate currently accounts only for todos, a queue chip,
  right-hand controls, and proceed.
- The chip reports both directions and opens to a list of blocked-by and blocks
  entries with each entry's title and state, each navigable to that task.
- Failed predecessors are visually distinguished from pending ones in the list.
- The chip returns null when the task has no edges in either direction, matching
  `PRStatusChip`'s own null-when-not-applicable contract.
- Mobile uses the drawer pattern, not a hover card, mirroring
  `PRStatusChipDrawer` / `usesMobileDrawer` in
  `components/github/pr-status-chip.tsx`.
- The chip reads `dependsOn` / `blocks` from the task payload; it does not fetch
  each related task individually.
- A blocked badge renders on the shared task card next to the existing
  `queuedForStepId` badge. It shows the predecessor count and distinguishes
  `pending` from `failed`. Hovering or tapping it lists the predecessor titles
  and their states.
- Both badges can appear on one card without truncating either, on desktop and
  on a narrow mobile viewport, with no horizontal page overflow and no
  hover-only disclosure of the blocked state itself.
- The Kanban task detail view mounts **no** edge editor. Dependencies are
  declared in the create dialog or over MCP, so every detail-view surface that
  reads them (card badge, chip, chip list) is read-only and no `BlockersPicker`
  is repointed at the task-scoped routes.
- The task-create dialog gains a "Depends on" field and a "Start automatically
  when unblocked" checkbox, submitting `blocked_by` and `start_when_unblocked`.
  The checkbox is disabled when no dependency is selected.
- The Kanban card context menu gains an "Add dependency" entry, following the
  `kanban-card-link-submenu.tsx` pattern.
- All new copy goes through `t()` / `<Trans>`; no hardcoded literal, including
  inside SCREAMING_CASE config tables, which are reviewed by eye. Every path
  touched here is appended to `i18nGuardFiles` in `eslint.i18n.options.mjs` in
  the same change. `t()` is never called at module scope.
- The pseudo-locale (Settings → General → Appearance) shows no English text in
  any surface added by this task.
- Mobile parity: the picker and the create-dialog field are reachable and usable
  on a narrow viewport using native mobile interaction patterns, not a
  compressed desktop layout.

## TDD sequence

1. Failing tests: DTO/store conversion preserves the five fields from boot and
   from a `task.updated` event with `fields: ["dependencies"]`.
2. Failing test: the blocked badge renders count and reason, and coexists with
   the queued badge.
2a. Failing test: `shouldRenderChatStatusBar` returns true for a task whose only
   status content is the dependency chip.
2b. Failing test: the chip lists blocked-by and blocks entries with states,
   distinguishes a failed predecessor, and renders null with no edges.
2c. Failing test: the chip appears in both `chat-status-bar` and
   `passthrough-status-row`.
3. Failing test: the picker creates and removes an edge optimistically.
4. Failing test: a `409` with a `cycle` body renders the formatted cycle path
   and reverts the optimistic edge.
5. Failing test: the create dialog submits `blocked_by` and
   `start_when_unblocked`, and the checkbox is disabled with no selection.
6. Failing test: the context-menu entry opens the picker for the right task.
7. Implement types, mappers, badge, picker mount, dialog fields, and menu entry.

## Verification

```bash
cd apps/web
pnpm test -- --run components/kanban-card-content components/task/simple/components/blockers-picker components/task-create-dialog components/task/chat/chat-input-area components/task/dependency-chip lib/ws/handlers/kanban
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

Use `pnpm test` / `pnpm run typecheck`, not `pnpm exec` — `exec` skips the
generator pre-hooks and produces spurious generated-JSON failures.

## Files likely touched

- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/ws/handlers/kanban.ts`
- `apps/web/components/kanban-card.tsx`, `kanban-card-content.tsx`
- `apps/web/components/kanban-card-context-menu.tsx`
- `apps/web/components/task/simple/components/blockers-picker.tsx`
- `apps/web/components/task/dependency-chip.tsx` (new)
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/passthrough-toolbar.tsx`
- `apps/web/app/tasks/[id]/kanban-task-shell.tsx`
- `apps/web/components/task-create-dialog*.tsx`
- `apps/web/lib/api/domains/` (task-scoped dependency client)
- `apps/web/eslint.i18n.options.mjs`
- `apps/web/public/locales/**` (or the repo's catalog location)
- focused `*.test.tsx` / `*.test.ts` files

## Dependencies

Task 01 — needs the task-scoped routes and the five DTO fields. It does not need
Tasks 02–05, so it may start as soon as 01 lands, alongside Task 05.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record the badge layout decision for the two-badge case,
the i18n paths appended to the guard list, the pseudo-locale check result, and
test results in this file and `plan.md`.
