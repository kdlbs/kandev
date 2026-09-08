---
id: "04-visibility-sync-hook"
title: "Visibility-sync hook"
status: done
wave: 3
depends_on: ["03-todos-panel-registration"]
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 04: Visibility-sync hook

Wire the `showTodoListPanel` preference to the live Dockview tree via a new
conditional-panel sync hook modeled on `useSyncReviewPanel`, without its
review-identity/`reviewsLoaded`/session-suppression concerns.

- **Acceptance:**
  1. `resolveConditionalTodoPanelAction` returns `"add"` when the setting is
     on and the panel is absent, `"remove"` when the setting is off and the
     panel is present, `"none"` otherwise — covered by a unit test for all
     four combinations (plus settings-hydration gating).
  2. `useSyncTodoPanel()`, invoked from `dockview-desktop-layout.tsx` beside
     `useSyncReviewPanel()`, adds an inactive `todos` tab to the active task's
     live layout when the setting turns on (in the custom Default's
     configured group/index when configured, else beside Files/Changes),
     and removes it when the setting turns off — without waiting for a page
     reload and without mutating the saved layout/profile data.
  3. Toggling the setting off while a task has a `todos` panel that came from
     a saved layout (not from this sync) still removes it at runtime, per the
     spec's "true, unconditional visibility gate" requirement; the saved
     layout's `todos` entry itself is untouched.
- **Verification:** `cd apps/web && pnpm run typecheck && pnpm exec vitest run components/task/dockview-todo-panel-sync.test.ts`
- **Files touched (reconciled with actual diff):**
  - `apps/web/components/task/dockview-todo-panel-sync.ts` (new)
  - `apps/web/components/task/dockview-todo-panel-sync.test.ts` (new)
  - `apps/web/components/task/dockview-desktop-layout.tsx`
  - `apps/web/src/locales/en/common.json` / `apps/web/src/locales/pseudo/common.json`
    (new `todos` title key for the conditionally-added panel, mirroring
    `prDetails`)
- **Dependencies:** Task 03 (the `todos` panel type and `PANEL_REGISTRY` entry
  must exist before the sync hook can add/remove it).
- **Parallelism:** `sequential`.
- **Inputs:** Spec's What section (visibility-sync hook description, the
  "true gate" and "no closed-for-session memory" requirements) and Scenarios;
  plan's Frontend → "Visibility-sync hook" subsection and Risks (the
  double-`requestAnimationFrame` timing requirement);
  `apps/web/components/task/dockview-review-panel-sync.ts` in full (the exact
  structural and timing template); `apps/web/components/task/dockview-desktop-layout.tsx:39-41,411`
  (invocation site).

## Results

Implemented `resolveConditionalTodoPanelAction`/`resolveConfiguredTodoPanelPlacement`/
`syncConditionalTodoPanel`/`useSyncTodoPanel` in a new
`dockview-todo-panel-sync.ts`, mirroring `dockview-review-panel-sync.ts`'s
structure and the same double-`requestAnimationFrame` deferred-effect pattern
and `useDockviewStore.getState()` live-read pattern in `useSyncReviewPanel`,
but with two deliberate simplifications: no review-identity/params-sync
concept (todos has nothing to reconcile once added — it's a boolean, not a
per-instance payload) and no `wasOffered`/`sessionStorage` closed-for-session
suppression (out of scope per spec — the preference is the sole authoritative
on/off action, unlike PR Details where *review linkage*, not a deliberate
user action, re-triggers the sync). Added a `settingsLoaded` gate (mirroring
PR Details' `reviewsLoaded`) so a cold-loading task doesn't flash-remove a
panel materialized from its saved layout before `userSettings` hydrates.
Default placement falls back to `RIGHT_TOP_GROUP` (beside Files/Changes, per
the spec's "right panel" requirement) rather than PR Details' Agent-center
fallback, then to the center group only for layouts with no right column
(e.g. compact). Wired `useSyncTodoPanel()` into `dockview-desktop-layout.tsx`
beside the existing `useSyncReviewPanel()` call.

Wrote `dockview-todo-panel-sync.test.ts` (16 cases: the resolver's full
decision matrix including settings-hydration gating, configured-placement
resolution, `syncConditionalTodoPanel`'s add/remove/fallback/no-op paths)
before implementing; confirmed red (module not found) first, green after.
Caught and fixed two self-introduced mistakes mid-implementation via
mid-flight review: an extracted single-call wrapper function that violated
the repo's no-tiny-functions convention (inlined), and a duplicated JSDoc
block left over from that inlining edit (removed).

Commands and results:
- `pnpm exec vitest run components/task/dockview-review-panel-sync.test.ts components/task/dockview-todo-panel-sync.test.ts components/task/dockview-shared.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/dockview-panel-content.diff.test.tsx components/task/DockviewLayout.test.tsx` → 6 files, 52 tests passed (no regressions in the existing PR-details sync or dockview panel-content tests from adding the new hook/panel alongside them).
- `pnpm run typecheck` → clean.
- `pnpm run i18n:check` → `2123 key(s) referenced, 2407 en entries, 0 orphans, pseudo in sync`.
