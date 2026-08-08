---
id: "03-todos-panel-registration"
title: "Todos panel registration and rendering"
status: done
wave: 2
depends_on: ["02-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/ui/agent-todo-list-panel.md"
---

# Task 03: Todos panel registration and rendering

Register `todos` as a new reusable Dockview panel and render the agent's live
todo checklist inside it, reusing `TodoIndicator`'s existing row/status
rendering rather than duplicating it.

- **Acceptance:**
  1. `todos` is present in `REUSABLE_PANEL_IDS`, `KNOWN_PANEL_IDS`, and
     `PANEL_REGISTRY` (title "Todos", no custom tab component — normal
     closable default tab).
  2. `renderPanel(panelId, "todos", params)` renders the same
     pending/in-progress/completed/failed rows (same order, same icons) that
     `TodoIndicator`'s popover renders for a session with entries, and an
     empty state for a session with none, via a shared component extracted
     from `todo-indicator.tsx` (not a re-implementation).
  3. `TodoIndicator`'s own rendered output and existing tests are unchanged.
  4. No built-in layout preset (`defaultLayout`, `compactLayout`, `planLayout`,
     `previewLayout`, `vscodeLayout`) includes `todos` — it is registered but
     never auto-placed by a template.
- **Verification:** `cd apps/web && pnpm run typecheck && pnpm exec vitest run components/task/dockview-shared.test.tsx components/task/chat/`
- **Files touched (reconciled with actual diff):**
  - `apps/web/lib/state/layout-manager/constants.ts`
  - `apps/web/components/task/dockview-shared.tsx`
  - `apps/web/components/task/dockview-shared.test.tsx` (new)
  - `apps/web/components/task/chat/todo-indicator.tsx` (added
    `TodoIndicatorContent` to its export list only — no behavior change)
  - `apps/web/components/task/chat/use-chat-panel-state.ts` (added `export`
    to `useSessionTodoItems` only — no behavior change)
  - `apps/web/src/locales/en/chat.json` / `apps/web/src/locales/pseudo/chat.json`
    (new `noTodosYet` key for the panel's empty state, per the repo's i18n
    rule — regenerated pseudo via `node scripts/generate-pseudo-locale.mjs`)
- **Dependencies:** Task 02 (informational only; this task does not read
  `showTodoListPanel`).
- **Parallelism:** `sequential`.
- **Inputs:** Spec's What / Data model sections; plan's Frontend → "Reusable
  Todos panel registration" subsection; `PANEL_REGISTRY`/`REUSABLE_PANEL_IDS`/
  `KNOWN_PANEL_IDS`; `PlanContent`/`renderPanel`'s `"plan"` case as the
  placement template; `TodoIndicator`'s `TodoIndicatorContent`/`resolveStatus`;
  `useSessionTodoItems`.

## Results

Added `todos` to `REUSABLE_PANEL_IDS`, `KNOWN_PANEL_IDS`, and
`PANEL_REGISTRY` (`{ component: "todos", title: "Todos" }`, default tab —
normal close control, no custom tab component). Added `todos: PortalSlot` to
`dockviewComponents` and a `case "todos": return <TodosContent />;` branch in
`renderPanel`. `TodosContent` resolves `state.tasks.activeSessionId`, calls
the now-exported `useSessionTodoItems(sessionId, [])`, and either renders the
now-exported `TodoIndicatorContent` (identical rows/progress/status icons
`TodoIndicator` uses) or a translated (`chat:noTodosYet`) empty state when the
session has no todo entries. No preset (`presets.ts`) references `"todos"`,
confirmed by grep.

Wrote `dockview-shared.test.tsx` (mirroring the existing
`dockview-panel-content.diff.test.tsx` mocking pattern) before implementing;
confirmed red (`Unknown panel: todos`) first. After implementing, strengthened
the populated-panel test to assert row order, the shared "1/2 completed"
progress header, and per-row status semantics (completed row struck through,
in-progress row not) — not just row text — so the panel's reuse of
`TodoIndicatorContent`'s status semantics is directly verified.

Commands and results:
- `pnpm exec vitest run components/task/dockview-shared.test.tsx components/task/chat/ components/task/dockview-panel-content.diff.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/plan-tab.test.tsx` → 67 files, 640 tests passed (no regressions in `TodoIndicator`/chat area from widening its exports).
- `pnpm run typecheck` → clean (after catching and fixing a self-introduced
  import-editing mistake that briefly dropped the `MRDetailPanelComponent`
  import — restored before this run).
- `pnpm run i18n:check` → `2122 key(s) referenced, 2406 en entries, 0 orphans, pseudo in sync`.

**Addendum (found during Task 05's E2E pass):** `dockview-shared.tsx` backs
only the *Office* task layout (`app/office/tasks/[id]/office-dockview-layout.tsx`).
The main desktop task workbench (`/t/:id`, `dockview-desktop-layout.tsx` +
`dockview-panel-content.tsx`) keeps its own parallel `components`/`renderPanel`
registry and never got the matching `todos` entries in this task, so the
registration/rendering acceptance criteria above held for Office but not for
the actually-in-scope desktop workbench. Fixed in Task 05; see that task's
Results for the full explanation.
