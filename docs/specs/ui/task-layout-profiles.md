---
status: building
created: 2026-07-19
owner: kandev
---

# Task Layout Profiles

## Why

Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, or who prefer a different Files and Changes arrangement, need a durable settings surface that does not disturb layouts already customized for individual tasks.

## What

- `Settings > General > Layouts` is the central manager for reusable desktop task-layout profiles and is reachable on desktop and mobile settings navigation.
- The page lists the built-in Default, Plan Mode, Preview Mode, and VS Code layouts as stable rows. A user edits a built-in directly; Kandev stores a hidden override while keeping the built-in row selected and marks it `Customized`. Reset removes the override and restores the code-defined layout.
- A user can create, rename, duplicate, edit, delete, and select the default custom profile. Names must be non-empty; profile IDs must be unique.
- Exactly one layout is effective as the user default. A saved profile, including a reserved built-in override, marked `is_default` wins; when none is marked, the built-in Default layout is effective.
- The visual editor supports one instance of each reusable panel: Agent, Files, Changes, Terminal, Plan, Browser, and VS Code. Agent is required and cannot be removed.
- Selecting a tab makes it active and shows contextual controls next to its split. Users can reorder or remove the tab, move it between groups, create splits, and move, merge, or resize the selected split. Adding a missing panel remains a separate floating action. Every editor action provides a hover/focus description.
- Layout changes use the shared Settings floating save control and navigation guard. The page does not render its own Save or Cancel buttons.
- Removing Terminal from the effective default prevents the default terminal panel and its backing user shell from being created when a fresh task environment is first opened.
- Applying a layout preserves every configured reusable panel regardless of whether the task has repositories. Panels without applicable content remain available and show their normal empty state.
- A changed default applies to task environments that have no saved task-specific layout and to an explicit Reset Layout action. It does not overwrite an existing task-specific layout merely because the setting changed.
- Switching between tasks whose sessions share one task environment atomically replaces task-owned Agent tabs in their existing group. The handoff preserves the live split tree and proportions instead of briefly emptying the group or rebuilding the environment layout.
- Desktop Agent-tab reconciliation preserves both the selected tab in each existing group and the globally focused panel. Replacing a selected Chat placeholder with the active session keeps Agent selected in that group without transiently selecting a neighboring Plan tab, while focus in another group such as Files or Changes remains unchanged. A valid non-Agent tab that was already selected in the Agent group remains selected.
- The existing workbench layout menu continues to apply built-in and custom profiles and save the current workbench as a custom profile. Profile mutations from either surface remain consistent after the user-settings response is received.
- Layout-profile editing is usable with pointer, keyboard, and touch input. On narrow settings viewports, profile management and all editor commands remain reachable without horizontal page scrolling.
- Layout profiles configure the desktop Dockview workbench only. Mobile and tablet task-detail layouts retain their existing behavior.
- The default right-side workbench column is responsive: its Files, Changes, and Terminal panels resize together from the current desktop workbench width whenever the display changes.
- A right-column resize performed through the desktop sash is an explicit per-task-environment preference. It persists across reloads and display changes, while still respecting the current screen's safety cap.

## Data model

Layout profiles remain in the backend-owned `users.settings.saved_layouts` JSON value; no schema migration or second durable store is introduced.

`SavedLayout`

| Field | Type | Constraint |
|---|---|---|
| `id` | string | Non-empty and unique within the user's list |
| `name` | string | Non-empty after trimming |
| `is_default` | boolean | At most one saved profile is `true` |
| `layout` | JSON object | Reusable `LayoutState` payload |
| `created_at` | ISO-8601 string | Preserved when editing; newly assigned when creating or duplicating |

The built-in layouts are code-defined templates. A customization is stored in `saved_layouts` under the reserved stable ID `layout-override-<built-in-id>`, but is hidden from the Custom list and presented as the same built-in row. Reserved overrides participate in the same single-`is_default` invariant as custom profiles. A Default override replaces the code-defined Default as the effective default only when that override owns `is_default`; editing it claims the default when no saved profile currently owns it and otherwise preserves the existing custom default. If no saved profile has `is_default: true`, the code-defined Default template is the effective default. Resetting a built-in removes only its reserved override.

The editor persists the existing declarative `LayoutState`: ordered columns contain ordered groups, groups contain ordered panels and an active panel, and captured tree/size data preserves split placement and proportions. New editor-created profiles use only the reusable panel registry. A legacy profile with an unreadable layout remains listed for rename, duplication, deletion, or default removal, but cannot enter the visual editor or become a new default until replaced with a valid reusable layout.

Task-specific restored layouts remain device-local environment state and take precedence over the user default. They are not copied into or overwritten by layout-profile edits. The serialized Dockview layout preserves panel structure and transient geometry. A companion environment-scoped preference stores a raw right-column width only after a genuine user sash drag; legacy layouts and layouts without that preference are responsive defaults rather than manual overrides.

## API surface

No new endpoint is introduced.

- `GET /api/v1/user/settings` returns `settings.saved_layouts`.
- `PATCH /api/v1/user/settings` accepts `saved_layouts` as a complete replacement list and returns the updated user settings.
- A `saved_layouts` update returns `400 Bad Request` when it exceeds the existing limit, contains an empty ID or name, contains duplicate IDs, or marks more than one saved profile, including reserved overrides, as default.

The frontend treats the returned settings payload as authoritative after each successful mutation.

## Failure modes

- If a profile save fails, the editor keeps the unsaved draft, reports the error, and leaves the previously persisted profiles/default unchanged.
- If a saved default layout is unreadable or contains no usable Agent panel, the workbench falls back to the built-in Default layout instead of rendering a broken or empty workbench.
- If a legacy profile cannot be opened by the visual editor, the page identifies it as unavailable for editing and does not silently rewrite its payload.
- Browser and VS Code panels in the settings preview do not launch, download, connect to, or authenticate external processes. Their normal runtime behavior begins only when the profile is applied in a task.
- Deleting the current custom default requires confirmation and makes the built-in Default layout effective.

## Persistence guarantees

- Custom profiles and the selected custom default survive browser and Kandev restarts through backend user settings and are portable across the user's devices.
- An unsaved editor draft does not survive navigation or restart.
- Per-task layout state continues to use its existing environment-scoped persistence and is not made portable by this feature.
- A saved default right-column geometry adapts to the current workbench width on reload, monitor switch, and return to a wider monitor. A manual right-column width keeps its raw requested width across those events and is only clamped while the current screen cannot accommodate it.
- A task handoff within the same environment does not persist an intermediate panel-removal state or change the root split orientation.
- Completing a normal, non-maximized desktop task-layout restore re-establishes a live Agent panel for the active session before the workbench is revealed. If the restored center group is empty, Agent is inserted there and activated; if another valid saved center tab such as Plan is active, that tab remains active.
- Replacing the generic Chat placeholder with a session-owned Agent panel preserves the placeholder group's selected content and the workbench's globally focused panel; internal tab insertion, removal, or ordering does not acknowledge an unseen Plan.

## Scenarios

- **GIVEN** the user opens General settings on desktop or mobile, **WHEN** they select Layouts, **THEN** the built-in templates, custom profiles, and effective default are visible.
- **GIVEN** the built-in Default layout, **WHEN** the user removes Terminal and saves with the shared floating control, **THEN** the same Default row is marked `Customized` and its hidden default override persists without requiring a duplicate step.
- **GIVEN** a customized built-in layout, **WHEN** the user chooses Reset and saves, **THEN** its hidden override is removed and the original code-defined layout is restored.
- **GIVEN** a customized built-in layout, **WHEN** the user selects that built-in from the task workbench layout menu, **THEN** the saved override is applied instead of the original code-defined template.
- **GIVEN** a valid custom profile, **WHEN** the user reorders tabs or moves a panel into a new split and saves, **THEN** reopening the profile shows the same tab order, active tab, split order, and proportions.
- **GIVEN** a default profile without Terminal and a task environment with no saved layout, **WHEN** the user first opens that task, **THEN** the workbench has no Terminal tab and no default user shell is created.
- **GIVEN** an existing task with a task-specific layout, **WHEN** the user changes the default profile and returns to that task, **THEN** the task-specific layout is unchanged.
- **GIVEN** an existing task with a task-specific layout, **WHEN** the user chooses Reset Layout, **THEN** the latest effective default profile replaces that task's layout.
- **GIVEN** two tasks whose active sessions share one task environment and a desktop workbench with Agent in the center and Files or Changes above Terminal on the right, **WHEN** the user switches between those tasks, **THEN** the incoming Agent replaces the outgoing Agent in the same group, the right column remains vertically split, the root remains horizontally split, and the same geometry survives reload.
- **GIVEN** a normal desktop task layout restore completes while the active session's Agent panel is absent, **WHEN** the center group is empty, **THEN** the Agent panel is restored into that group and activated before the workbench is shown; when Plan or another valid saved center tab is active, restoring Agent does not steal focus from it, and a deliberately maximized non-Agent group remains unchanged.
- **GIVEN** a desktop task layout whose Agent group has Chat selected beside an unseen Plan while Files or Changes owns global focus, **WHEN** session reconciliation replaces Chat with the active session's Agent panel, **THEN** Agent remains selected in its group, the globally focused Files or Changes panel remains focused, Plan is never selected or marked seen, and the user is not switched to Plan.
- **GIVEN** a task environment whose right column has never been manually resized, **WHEN** its desktop workbench moves from a large monitor to a laptop-sized workbench and back, **THEN** the Files, Changes, and Terminal column follows the default ratio at each width and returns to the large-workbench ratio.
- **GIVEN** a task environment whose right column was manually resized through its desktop sash, **WHEN** the workbench moves between large and laptop-sized displays, **THEN** that requested width is restored whenever it fits and is temporarily clamped only to preserve the current screen's minimum center width.
- **GIVEN** a custom default profile, **WHEN** the user deletes it and confirms, **THEN** the built-in Default becomes effective.
- **GIVEN** a profile draft with Agent removed, duplicate reusable panels, or an empty group, **WHEN** the user attempts to save, **THEN** saving is blocked and the invalid locations are identified.
- **GIVEN** a backend save failure, **WHEN** the user saves a profile edit, **THEN** the draft remains available and the previous persisted layout stays selected.
- **GIVEN** a legacy unreadable saved profile, **WHEN** the Layouts page loads, **THEN** the profile remains available for non-editor management, is marked unavailable for visual editing, and is not silently modified.

## Out of scope

- Customizing mobile or tablet task-detail layouts.
- Changing the global app sidebar width or other layout-profile split proportions.
- Forcing a changed default onto existing task-specific layouts without Reset Layout.
- Configuring task-specific panels such as individual files, diffs, commits, pull requests, extra sessions, or extra terminals.
- Mutating the code-defined built-in definitions; direct edits are persisted as hidden user overrides.
- Sharing profiles between users or scoping profiles to a workspace, repository, agent, or executor.
