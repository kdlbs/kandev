---
status: shipped
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
- The existing workbench layout menu continues to apply built-in and custom profiles and save the current workbench as a custom profile. Profile mutations from either surface remain consistent after the user-settings response is received.
- Layout-profile editing is usable with pointer, keyboard, and touch input. On narrow settings viewports, profile management and all editor commands remain reachable without horizontal page scrolling.
- Layout profiles configure the desktop Dockview workbench only. Mobile and tablet task-detail layouts retain their existing behavior.

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

Task-specific restored layouts remain device-local environment state and take precedence over the user default. They are not copied into or overwritten by layout-profile edits.

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

## Scenarios

- **GIVEN** the user opens General settings on desktop or mobile, **WHEN** they select Layouts, **THEN** the built-in templates, custom profiles, and effective default are visible.
- **GIVEN** the built-in Default layout, **WHEN** the user removes Terminal and saves with the shared floating control, **THEN** the same Default row is marked `Customized` and its hidden default override persists without requiring a duplicate step.
- **GIVEN** a customized built-in layout, **WHEN** the user chooses Reset and saves, **THEN** its hidden override is removed and the original code-defined layout is restored.
- **GIVEN** a customized built-in layout, **WHEN** the user selects that built-in from the task workbench layout menu, **THEN** the saved override is applied instead of the original code-defined template.
- **GIVEN** a valid custom profile, **WHEN** the user reorders tabs or moves a panel into a new split and saves, **THEN** reopening the profile shows the same tab order, active tab, split order, and proportions.
- **GIVEN** a default profile without Terminal and a task environment with no saved layout, **WHEN** the user first opens that task, **THEN** the workbench has no Terminal tab and no default user shell is created.
- **GIVEN** an existing task with a task-specific layout, **WHEN** the user changes the default profile and returns to that task, **THEN** the task-specific layout is unchanged.
- **GIVEN** an existing task with a task-specific layout, **WHEN** the user chooses Reset Layout, **THEN** the latest effective default profile replaces that task's layout.
- **GIVEN** a custom default profile, **WHEN** the user deletes it and confirms, **THEN** the built-in Default becomes effective.
- **GIVEN** a profile draft with Agent removed, duplicate reusable panels, or an empty group, **WHEN** the user attempts to save, **THEN** saving is blocked and the invalid locations are identified.
- **GIVEN** a backend save failure, **WHEN** the user saves a profile edit, **THEN** the draft remains available and the previous persisted layout stays selected.
- **GIVEN** a legacy unreadable saved profile, **WHEN** the Layouts page loads, **THEN** the profile remains available for non-editor management, is marked unavailable for visual editing, and is not silently modified.

## Out of scope

- Customizing mobile or tablet task-detail layouts.
- Forcing a changed default onto existing task-specific layouts without Reset Layout.
- Configuring task-specific panels such as individual files, diffs, commits, pull requests, extra sessions, or extra terminals.
- Mutating the code-defined built-in definitions; direct edits are persisted as hidden user overrides.
- Sharing profiles between users or scoping profiles to a workspace, repository, agent, or executor.
