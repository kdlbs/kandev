---
status: current
system: tasks
requirements:
  - REQ-TASKS-CREATION-AUTO-FOCUS-001
---

# Task creation auto-focus design

## Boundary and existing behavior

The task system owns completion of task creation. Reuse the existing per-user
settings contract; this adds one preference, without a new persistence system,
permission boundary, runtime flag, or task-creation API parameter.

`task-create-dialog-submit.tsx` computes `willNavigate` for passthrough and plan
launches. Sidebar, task-header, canvas, and phone switcher success callbacks can
also navigate or select tasks. `willNavigate: false` currently means the caller
may navigate, so it cannot represent suppression.

## Preference contract

Add `AutoFocusNewTasks` / `auto_focus_new_tasks` to backend user settings and
`autoFocusNewTasks` to frontend state. The effective default is **true**. Extend
model defaults, DTO responses and pointer-valued patches, service update and
response mapping, SQLite JSON marshal/scan, HTTP/WS handler mappings, Go boot
payload, frontend HTTP types, SSR mapping, and store defaults. Decode missing
legacy JSON as true while preserving explicit false; partial updates to other
fields must preserve the saved value. No SQL column migration is needed.

Use the existing `PreventAutoStartAgentOnOpen` path as the wiring exemplar,
but do not inherit its false default. Extend settings discovery in
`internal/settingscatalog/defaults.go` and
`lib/settings-discovery/catalog/preferences.ts`; regenerate both snapshots with
`go run ./cmd/settings-catalog` from `apps/backend`.

## Creation completion policy

At submission, snapshot the effective saved preference. Propagate a separate
optional `autoFocus` boolean in the shared creation success metadata, including
the no-agent submission path. Undefined preserves existing callback behavior.
Use `autoFocus !== false` at consumers; do not repurpose `willNavigate`.

Always perform successful completion bookkeeping, cache upserts, draft clearing,
last-used updates, and dialog dismissal. Gate only route changes, active task
and session changes, preview changes, and layout switching. Keep `onSuccess`
delivery enabled. Compute `willNavigate` from both policy and existing launch
rules so enabled behavior still avoids duplicate navigation.

Audit every `TaskCreateDialog` success consumer and forwarding type, including:

- `components/app-sidebar/app-sidebar-new-task-item.tsx`.
- `components/task/dockview-header-actions.tsx`, `new-task-button.tsx`, and `new-task-dropdown.tsx`.
- `components/task/mobile/session-task-switcher-sheet-hooks.ts` and its dialogs.
- `components/canvas/canvas-task-create-launcher.tsx`.
- `components/kanban-board.tsx` and `handleDialogSuccess` in `hooks/domains/kanban/use-kanban-actions.ts` (cache-only; no focus change needed).
- GitHub, GitLab, Jira, Linear, and Azure DevOps task launchers: retain association/cache work, gate their final navigation.
- Improve Kandev creation forwards policy through its success wrapper to sidebar-footer and app-navigation callers.

`activatePlanMode` in `task-create-dialog-helpers.ts` currently combines
session-keyed plan state/context initialization and router navigation. Separate
these responsibilities or give navigation an explicit option: initialize the
new session's plan state while suppressing navigation when requested. Do not
change the viewed task's layout or active session. Existing additional-session
and edit flows keep their current behavior.

This policy does not add focus to entry points that currently stay put. Do not
introduce global task-created-event selection. Explicit user selection after
creation continues through existing navigation.

## Settings and mobile presentation

Add a small settings card next to existing task-action preferences, wired through
`useSettingsSaveContributor`. Reuse saved/draft reconciliation and failure
propagation from `prevent-auto-start-agent-settings.tsx`. Publish the effective
store value only after a successful save; discard restores the baseline.

Copy: “Auto-focus new tasks”. Help: “Open newly created tasks automatically.
Turn this off to stay on your current view. Tasks and agents still start as
requested.” Localize via `t()` in English, Portuguese, and Chinese catalogs;
generate Traditional Chinese with `pnpm run i18n:zh-hant`.

Desktop and phone share the inline card on `/settings/general/task-actions`.
The closest settings exemplar is the existing auto-start preference; the phone
task navigation exemplar is `session-task-switcher-sheet.tsx`, which uses an
inset drawer. Keep the settings page's existing scroll owner and shared
safe-area-aware save control. Allow help text to wrap; use a real 44px touch
hit area on phone/coarse pointers while keeping desktop density. No new overlay
is needed for one persistent boolean. Creation shells retain their existing
phone composition. When automatic opening is disabled, the shared dialog captures
the opening control before child input-focus effects and restores it on dismissal
if it remains mounted. An explicit caller focus-return ref takes precedence.

## Verification mapping

| Criteria | Evidence |
| --- | --- |
| 001.1, 001.4 | Backend default/round-trip/partial-update tests; boot and SSR mapping tests; settings save/discard/failure component tests |
| 001.2, 001.3 | Submit and caller tests asserting no navigation/selection/layout mutation while creation, cache updates, and plan initialization continue |
| 001.1–001.5 | Desktop and mobile E2E for default on, saved off, reload, background creation, and manual opening; phone hitbox/overflow and focus return |

## Failure and operational behavior

Keep existing creation and settings error reporting. No new logging or metrics
are needed for a user presentation preference. The highest compatibility risks
are losing explicit false during hydration, skipping plan setup, and a caller
reintroducing navigation after the submit helper suppresses it.

## Related contracts

- [Requirements](../requirements/creation-auto-focus.md)
- [Prevent agent auto-start on open](../requirements/prevent-agent-autostart-on-open.md)
- [Mobile navigation](../../ui/requirements/mobile-task-navigation.md)
- [Implementation plan](../../../plans/task-creation-auto-focus/plan.md)
