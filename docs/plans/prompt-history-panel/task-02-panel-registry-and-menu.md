---
id: "02-panel-registry-and-menu"
title: "Panel registry and menu"
status: done
wave: 2
depends_on: ["01-prompt-duration-domain"]
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-history-panel.md"
---

# Task 02: Panel registry and menu

## Acceptance

- The `prompt-history` panel renders in both workbench registries (`dockview-panel-content.tsx` desktop and `dockview-shared.tsx` Office) from one shared content component; `PANEL_REGISTRY` resolves its localized title via `panelTitle("prompt-history")`.
- Dockview component maps accept the new component: `prompt-history: PortalSlot` is added to BOTH maps — `dockviewComponents` in `dockview-shared.tsx` and the `components` map in `dockview-desktop-layout.tsx` (every panel component is listed there; `VALID_COMPONENTS` derives from the shared map and the desktop layout derives its own valid set). Without these, `addPanel` has no PortalSlot and restored layouts are rejected as invalid components. Valid-component/map-membership tests assert acceptance in both workbenches, in addition to the renderer tests. Desktop seam: `dockview-desktop-layout.tsx` keeps `components` and `VALID_COMPONENTS` module-private (Office exports `VALID_COMPONENTS` from `dockview-shared.tsx`), so this task must add an exported seam — export `VALID_COMPONENTS` from `dockview-desktop-layout.tsx` (mirroring Office) or an `isValidDockviewComponent(component)` helper — and the desktop membership test asserts that exported contract against the real map.
- Stable test IDs (owned by this task, consumed by Task 04): `prompt-history-panel` (root), `prompt-history-row-<index>` (newest-first index), `prompt-history-expand-<index>`, `prompt-history-expanded-box-<index>`, `prompt-history-jump-<index>` (arrow), `prompt-history-duration-<index>` (the duration value element, so the E2E can assert the exact `formatPromptDuration` output including `0s` instead of a truthiness check).
- Layout-manager registration is complete, mirroring Todos: `prompt-history` is added to `REUSABLE_PANEL_IDS` and `KNOWN_PANEL_IDS` in `apps/web/lib/state/layout-manager/constants.ts` (without this, Settings > Layouts cannot offer it, layout capture/`filterEphemeral` drops it, and restore skips canonical-title normalization — `serializer.ts` keys off `KNOWN_PANEL_IDS`). Layout tests cover: presence in both sets, capture/restore survival, and `panelTitle`/`canonicalPanelTitle` resolution. Concrete homes: extend `apps/web/lib/state/layout-manager/panel-titles.test.ts` (exists) and create `apps/web/lib/state/layout-manager/serializer.test.ts` (NEW — no serializer test exists at HEAD; the `dockview-layout-restore.test.ts` under `components/task/` is the component-level restore sanitizer, a different surface).
- The "+" menu (`AddPanelMenuItems`) shows a Prompt history row that opens the panel in the invoking group (mirrors the Todos row), guarded by the same `!state.isPassthrough` check as the Plan and Todos rows — in passthrough sessions the row is absent (no transcript exists to navigate to).
- Each row renders newest-first with: arrow button, single-line truncated text (CSS `truncate`), compact relative send time (`formatRelative` — compact ladder, NOT `formatRelativeTime` which renders full phrases), duration, and an expand chevron visible when the collapsed text overflows horizontally (`scrollWidth > clientWidth`, re-measured on width changes via ResizeObserver) OR when the row is currently expanded — visibility is `hasCollapsedOverflow || expanded`, so the only collapse control never disappears right after expanding (the expanded text wraps and stops reporting horizontal overflow).
- Expanded rows show the full prompt UNTRUNCATED (wrapping text) inside a box capped at 40 % of the panel root's `clientHeight` (measured through a component ref; `PanelRoot` emits no `data-panel-kind` by default, so no ancestor selector), with `overflow-y-auto`; the cap re-measures on panel resize and falls back to `40vh` when no measurable container exists (root `clientHeight` 0 / isolated render). `aria-expanded` toggles.
- Accessibility contract is observable and tested: the send time is a `<time dateTime title>` element (visible text = compact relative; `title` = absolute time); the arrow button carries the localized `task:scrollToPrompt` `aria-label`; the chevron carries the localized `task:expandPrompt`/`task:collapsePrompt` `aria-label` matching `aria-expanded`.
- The arrow is wired through an injected callback seam (the `scrollTranscriptToMessage` action does not exist until Task 03): the component accepts `onNavigateToPrompt?: (messageId: string) => void` and the row arrow invokes it with the row's `messageId`. The REAL wiring — binding the callback to `scrollTranscriptToMessage(sessionId, messageId, resolvedTitle)` and resolving `session.name || panelTitle("chat")` (custom / EMPTY-string / absent `name` cases) — is Task 03's, where the action exists.
- Empty state renders when the session has no user prompts; the panel root carries `data-testid="prompt-history-panel"` (the stable geometry anchor for the 40 % cap and E2E measurements). A PASSTHROUGH active session renders a passthrough empty state with zero rows and zero arrow controls (the menu guard cannot cover restored/already-open tabs — the panel is reusable and persisted in layouts, and no layout-side passthrough filtering exists).
- The panel follows the task's active session (`tasks.activeSessionId`, the same reactive read `todos-panel-content.tsx` uses): a task with multiple agent transcripts shows the ACTIVE session's prompts only, never a merged list; switching the active session (session tab, session dropdown, or auto handoff) re-derives the whole list. `useSessionMessages` fetches the sibling's messages on switch and `useSessionTurns` hydrates its turns lazily, so the new session's rows appear immediately and its durations land when its turns arrive (until then `null`); a null active session shows the empty state.
- `addPromptHistoryPanel` action is tested directly (`apps/web/lib/state/dockview-panel-actions.prompt-history-panel.test.ts`, NEW): `addSidePanel` receives id `prompt-history`, component `prompt-history`, the localized title, the invoking group (center fallback), and the placement options. The Task 02 menu test only proves the mocked callback fires, so this is the test that catches a wrong id/component/title/group.
- Both registries have renderer tests: the desktop copy (extend the `dockview-panel-content.todos.test.tsx` pattern) and the Office copy in `dockview-shared.test.tsx`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/prompt-history-panel-content.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/dockview-panel-content.todos.test.tsx components/task/dockview-shared.test.tsx components/task/dockview-desktop-layout.test.ts lib/state/dockview-panel-actions.prompt-history-panel.test.ts lib/state/layout-manager/panel-titles.test.ts lib/state/layout-manager/serializer.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:pseudo && pnpm run i18n:check
```

(`lib/state/layout-manager/serializer.test.ts` is a NEW file created in this task; `lib/state/layout-manager/dockview-layout-restore.test.ts` does not exist.)

## Files likely touched

- `apps/web/components/task/prompt-history-panel-content.tsx` (new; optional `onNavigateToPrompt?: (messageId: string) => void` prop — the callback seam for Task 03's store wiring)
- `apps/web/components/task/prompt-history-panel-content.test.tsx` (new)
- `apps/web/lib/state/layout-manager/constants.ts` (PANEL_REGISTRY entry + `REUSABLE_PANEL_IDS` + `KNOWN_PANEL_IDS`)
- `apps/web/lib/state/layout-manager/panel-titles.test.ts` (title resolution coverage)
- `apps/web/lib/state/layout-manager/serializer.test.ts` (NEW — capture/restore survival)
- `apps/web/components/task/dockview-panel-content.tsx` (renderer entry)
- `apps/web/components/task/dockview-shared.tsx` (renderer entry + `prompt-history: PortalSlot` in `dockviewComponents`)
- `apps/web/components/task/dockview-desktop-layout.tsx` (`prompt-history: PortalSlot` in the `components` map; export `VALID_COMPONENTS` or an `isValidDockviewComponent` helper)
- `apps/web/components/task/dockview-shared.test.tsx` (Office renderer + `VALID_COMPONENTS` membership assertion)
- `apps/web/components/task/dockview-desktop-layout.test.ts` (desktop component-map membership assertion against the exported seam)
- `apps/web/lib/state/dockview-store.ts` (`addPromptHistoryPanel` type)
- `apps/web/lib/state/dockview-panel-actions.ts` (`addPromptHistoryPanel` impl via `addSidePanel`)
- `apps/web/lib/state/dockview-panel-actions.prompt-history-panel.test.ts` (NEW — `addPromptHistoryPanel` focused test)
- `apps/web/components/task/dockview-add-panel-items.tsx` (+ its test)
- `apps/web/src/locales/en/task.json`, `apps/web/src/locales/pseudo/task.json` (SOLE OWNER of English keys + pseudo regeneration: `promptHistory`, `promptHistoryEmpty`, `expandPrompt`, `collapsePrompt`, `scrollToPrompt`, `durationUnitSeconds`, `durationUnitMinutes`, `durationUnitHours`; pt-pt/zh-cn advisory entries are Task 04's)

## Dependencies

Task 01 (`buildPromptHistoryEntries`).

## Parallelism

Sequential.

## Inputs

- Spec: `What` (rows, expand cap, chevron overflow rule), the resize scenario, and `Out of scope` (recorded phone product decision).
- Plan: `Frontend > 2. Panel content + registries + "+" menu` and `Tests > Panel content` / `Tests > Dockview registries`.
- Patterns: `apps/web/components/task/todos-panel-content.tsx` (shared content + empty state), `apps/web/components/task/chat/anchored-last-prompt-bar.tsx` (expand cap ResizeObserver pattern, `IconChevronDown/Up` — but its `useCanExpand` measures VERTICAL overflow for a two-line layout; this row is single-line so measure `scrollWidth > clientWidth`), `apps/web/components/task/dockview-add-panel-items.tsx` Todos row, `apps/web/lib/state/dockview-panel-actions.ts` `addTodosPanel`, `apps/web/components/task/panel-primitives.tsx` `PanelRoot`.
- Time display: `formatRelative` (`apps/web/lib/i18n/formats.ts`, compact `5m ago` ladder) inside `<time dateTime title>` (pattern from `apps/web/components/task/chat/messages/message-actions.tsx`).
- Mobile: recorded product decision (spec `Out of scope`): no phone entry point; the phone's transcript already provides per-prompt jump arrows and run durations. No `mobile-*.spec.ts` is required because no mobile surface changes.

## Component test checklist (beyond the acceptance list)

- Chevron visible when `scrollWidth > clientWidth` (mocked); recomputes after a controlled width change (ResizeObserver callback); STILL visible while the row is expanded (`aria-expanded="true"`, collapse label) even though the expanded text wraps and no longer overflows — a literal "only when overflow" implementation that removes the chevron on expand must fail; clicking it collapses the row.
- Expansion state keyed by `messageId`, NOT row index: expand an older prompt, prepend a newer prompt, assert the SAME message (by id) stays expanded (index-keyed implementations must fail); test IDs remain index-based for DOM lookup only.
- Duration units come from catalog keys via runtime `t()` as BARE LOCALIZED LABELS (`task:durationUnitSeconds: "s"`, `durationUnitMinutes: "m"`, `durationUnitHours: "h"` — count-free, so no `_one`/`_other` siblings are needed and the `check-inline-plurals` gate passes) injected into the pure `formatPromptDuration` helper, which APPENDS the numeric count; never hardcoded English in the new helper; en + pseudo keys added in this task's catalog work, and a component test asserts the duration text is composed as `${count}${label}` from the localized units.
- Expanded cap = 40 % of mocked root `clientHeight`, `overflow-y-auto`, wrapping text; ROOT-HEIGHT REMEASURE: render at H1, expand, assert 0.4×H1; change the mocked root `clientHeight` to H2, fire the captured root observer callback, assert 0.4×H2 (a one-time measurement must fail); NO measurable root (`clientHeight` 0) → `40vh` fallback asserted.
- Test IDs present on root, rows, expand buttons, expanded boxes, and arrows per the acceptance list.
- Both workbench component maps (`dockviewComponents` / desktop `components`) accept `prompt-history`; `VALID_COMPONENTS` includes it.
- `<time>` has `dateTime` and `title`; with a PINNED fixed timestamp/now, the visible text asserts the compact `formatRelative` ladder exactly (`just now` / `5m ago` / `3h ago` / `2d ago` from the catalog — an implementation using the forbidden `formatRelativeTime` full phrases fails), and `title` is a parseable absolute-time rendering equal to `new Date(dateTime).toLocaleString()`.
- Arrow `aria-label` and chevron expand/collapse `aria-label` match the localized keys; `aria-expanded` toggles.
- Duration elements (`prompt-history-duration-<index>`) render EXACT `formatPromptDuration` output — including `0s` for a sub-second duration (a truthiness/absence-based assertion must fail if a valid `0s` is hidden or omitted).
- Empty state; arrow click invokes the INJECTED `onNavigateToPrompt` callback with the row's `messageId` (spy test; the real store wiring and title-resolution cases live in Task 03); duration text composed as `${count}${label}` from the localized units, with a pseudo-locale rendering (labels differ) proving the catalog path.
- ACTIVE-SESSION SWITCH (multi-transcript task): store state with TWO sessions' messages and turns; render with `tasks.activeSessionId` = A and assert A's rows; switch the store to B and assert the rows re-derive for B newest-first with NO A prompt remaining; switch to null and assert the empty state. (An implementation that keys to the route-initial session, or merges sessions, must fail.)
- Passthrough active session → passthrough empty state, zero rows, zero arrow controls.
- `dockview-add-panel-items.test.tsx`: row renders and opens the panel; with `isPassthrough: true` the row is absent.
- `dockview-panel-actions.prompt-history-panel.test.ts` (NEW): `addSidePanel` receives id/component `prompt-history`, localized title, invoking group (center fallback), placement options.

## Risks

- jsdom has zero client geometry and no deterministic ResizeObserver schedule: measurement tests MUST mock `scrollWidth`/`clientWidth` and the root `clientHeight`, and fire the observer callback explicitly, or they can pass while production falls back to `40vh` or shows a permanent chevron. The fallback case (no measurable root) must be tested explicitly, not assumed covered by the mocked-geometry cases.
- Both registries must gain the entry AND both renderer tests must cover it; the `dockview-panel-content.todos.test.tsx` comment documents that an entry landing in one registry without the other silently breaks one workbench.

## Output contract

Summary, files changed, exact commands and results, blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.
