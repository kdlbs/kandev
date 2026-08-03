---
status: building
created: 2026-07-19
owner: kandev
---

# Task Layout Profiles

## Why

Users can arrange and save the desktop task workbench only while a task is open, which makes the default layout difficult to discover or configure. Users who do not want an initial terminal, who prefer a different Files and Changes arrangement, or who want Pull Request details in a chosen pane need one durable layout surface that does not disturb layouts already customized for individual tasks. Panel placement belongs to the layout itself instead of a separate global appearance preference.

## What

- `Settings > General > Layouts` is the central manager for reusable desktop task-layout profiles and is reachable on desktop and mobile settings navigation.
- The page lists the built-in Default, Plan Mode, Preview Mode, and VS Code layouts as stable rows. A user edits a built-in directly; Kandev stores a hidden override while keeping the built-in row selected and marks it `Customized`. Reset removes the override and restores the code-defined layout.
- A user can create, rename, duplicate, edit, delete, and select the default custom profile. Names must be non-empty; profile IDs must be unique.
- Exactly one layout is effective as the user default. A saved profile, including a reserved built-in override, marked `is_default` wins; when none is marked, the built-in Default layout is effective.
- The visual editor supports one instance of each reusable panel: Agent, Files, Changes, PR Details, Terminal, Plan, Browser, and VS Code. Agent is required and cannot be removed.
- PR Details is the canonical reusable `pr-detail` panel. Its position in the selected layout is a placement template, not an instruction to keep an empty runtime tab open. The tab is visible only while the active task has a linked GitHub pull request or GitLab merge request, and then renders that review through the existing provider-aware review surface.
- The code-defined Default and compact desktop layouts omit PR Details. Agent remains initially selected in the Default center group, and Files remains initially selected in the top-right Files and Changes group.
- When the active task gains a linked GitHub pull request or GitLab merge request, Kandev adds the canonical panel as an inactive tab in the group and tab index configured by the selected custom Default. If that layout does not configure PR Details, Kandev adds it beside the live Agent panel. The user's current tab remains selected.
- After review data hydrates, Kandev removes the canonical PR Details panel whenever the active task has no linked review, including panels materialized from a selected profile or restored task layout. The saved layout data remains unchanged so it can continue to provide placement when a review is linked.
- Closing a conditionally added PR Details panel suppresses automatic re-creation for that session. The user can still reopen a specific review explicitly or configure future linked-review placement through the layout editor.
- Runtime review-data synchronization updates the canonical panel's provider and review identity without moving an existing panel. Explicit layout placement therefore remains user-owned even while review content changes.
- Explicitly opening a specific pull request or merge request focuses a matching tab in place. A new task-specific review tab joins the canonical PR Details panel's configured group when that panel exists and otherwise falls back to the existing center group; opening never relocates an already-open tab.
- Selecting a tab makes it active and shows contextual controls next to its split. Users can reorder or remove the tab, move it between groups, create splits, and move, merge, or resize the selected split. Adding a missing panel remains a separate floating action. Every editor action provides a hover/focus description.
- Layout changes use the shared Settings floating save control and navigation guard. The page does not render its own Save or Cancel buttons.
- Removing Terminal from the effective default prevents the default terminal panel and its backing user shell from being created when a fresh task environment is first opened.
- Applying a layout preserves every configured reusable panel regardless of whether the task has repositories, except PR Details, whose runtime visibility is conditional on a linked review. Other panels without applicable content remain available and show their normal empty state.
- A changed default applies to task environments that have no saved task-specific layout and to an explicit Reset Layout action. It does not overwrite an existing task-specific layout merely because the setting changed.
- Returning to a desktop task restores its task-specific layout, then activates Changes when a meaningful Git or commit update was detected while that task was inactive. This pending-change attention overrides the saved active panel, but an ordinary reload with already-known changes preserves the saved active panel.
- Switching between tasks whose sessions share one task environment atomically replaces task-owned Agent tabs in their existing group. The handoff preserves the live split tree and proportions instead of briefly emptying the group or rebuilding the environment layout.
- Desktop Agent-tab reconciliation preserves both the selected tab in each existing group and the globally focused panel unless pending inactive-task changes deliberately activate Changes. Replacing a selected Chat placeholder with the active session keeps Agent selected in that group without transiently selecting a neighboring Plan tab, while focus in another group such as Files or Changes remains unchanged. A valid non-Agent tab that was already selected in the Agent group remains selected.
- Desktop task-environment restoration finalizes each group's selected tab only after the incoming task's Agent panels have been reconciled. A saved or default Agent selection is semantic: `chat` or a stale `session:<id>` resolves to the incoming active session's live Agent panel in that group. Stable non-Agent selections such as PR Details, Plan, Files, Changes, or a surviving task-specific panel remain exact, and the restored globally focused group is applied last.
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

The editor persists the existing declarative `LayoutState`: ordered columns contain ordered groups, groups contain ordered panels and an active panel, and captured tree/size data preserves split placement and proportions. New editor-created profiles use only the reusable panel registry. The canonical `pr-detail` ID is reusable; keyed panels such as `pr-detail|owner/repository/123` and `mr-detail|host/project/123` remain task-specific runtime tabs and are not accepted by the profile editor. A legacy profile with an unreadable layout remains listed for rename, duplication, deletion, or default removal, but cannot enter the visual editor or become a new default until replaced with a valid reusable layout.

Task-specific restored layouts remain device-local environment state and take precedence over the user default. They are not copied into or overwritten by layout-profile edits. The serialized Dockview layout preserves panel structure and transient geometry. A companion environment-scoped preference stores a raw right-column width only after a genuine user sash drag; legacy layouts and layouts without that preference are responsive defaults rather than manual overrides.

## API surface

No new endpoint is introduced.

No `pr_panel_placement` user setting or other second placement contract is introduced. `saved_layouts` remains the only portable source for where PR Details appears when a review is linked.

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
- Existing saved profiles and task-specific layouts are not rewritten to add or remove PR Details. The code-defined templates without PR Details affect fresh environments and explicit Reset Layout actions; custom profiles retain their configured placement while runtime review state controls visibility.
- A saved default right-column geometry adapts to the current workbench width on reload, monitor switch, and return to a wider monitor. A manual right-column width keeps its raw requested width across those events and is only clamped while the current screen cannot accommodate it.
- A task handoff within the same environment does not persist an intermediate panel-removal state or change the root split orientation.
- Completing a normal, non-maximized desktop task-layout restore re-establishes a live Agent panel for the active session before the workbench is revealed. If the restored center group is empty, Agent is inserted there and activated; if another valid saved center tab such as Plan is active, that tab remains active.
- Replacing the generic Chat placeholder with a session-owned Agent panel preserves the placeholder group's selected content and the workbench's globally focused panel; internal tab insertion, removal, or ordering does not acknowledge an unseen Plan.
- Task-environment selection state survives dynamic Agent panel IDs. When the selected Agent identity in a saved task layout or fresh effective default no longer exists verbatim, Kandev rebinds that selection to the incoming active session instead of retaining a neighboring panel selected by the outgoing task or by Dockview reconciliation. Exact non-Agent selections are never rewritten as Agent selections.

## Scenarios

- **GIVEN** the user opens General settings on desktop or mobile, **WHEN** they select Layouts, **THEN** the built-in templates, custom profiles, and effective default are visible.
- **GIVEN** the built-in Default layout, **WHEN** the user removes Terminal and saves with the shared floating control, **THEN** the same Default row is marked `Customized` and its hidden default override persists without requiring a duplicate step.
- **GIVEN** a customized built-in layout, **WHEN** the user chooses Reset and saves, **THEN** its hidden override is removed and the original code-defined layout is restored.
- **GIVEN** a customized built-in layout, **WHEN** the user selects that built-in from the task workbench layout menu, **THEN** the saved override is applied instead of the original code-defined template.
- **GIVEN** a valid custom profile, **WHEN** the user reorders tabs or moves a panel into a new split and saves, **THEN** reopening the profile shows the same tab order, active tab, split order, and proportions.
- **GIVEN** the code-defined Default layout and a task with no linked review, **WHEN** a fresh desktop task environment opens or returns from Plan Mode, **THEN** Agent is selected, no PR Details tab is present, and Files remains selected in the top-right Files and Changes group.
- **GIVEN** a compact desktop task environment and a task with no linked review, **WHEN** the built-in compact layout is applied, **THEN** its single workbench group does not contain PR Details.
- **GIVEN** a selected layout without PR Details, **WHEN** the active task gains a linked GitHub pull request or GitLab merge request, **THEN** PR Details appears as an inactive tab beside Agent and renders that review without stealing focus.
- **GIVEN** a user moves PR Details to another group in the Layout editor and saves that profile, **WHEN** the profile is used for a fresh task with no linked review, **THEN** no PR Details tab is visible; **WHEN** that task gains a linked review, **THEN** the canonical panel opens as an inactive tab in the saved group without a separate appearance setting.
- **GIVEN** PR Details is present but the active task has no linked pull request or merge request, **WHEN** review data settles, **THEN** the runtime panel is removed while its placement remains in the saved profile.
- **GIVEN** Kandev conditionally added PR Details for a linked review, **WHEN** the user closes that tab and review data resynchronizes in the same session, **THEN** Kandev does not recreate the tab automatically.
- **GIVEN** Kandev showed PR Details for one task, **WHEN** the active task changes to one without a linked review, **THEN** no canonical PR Details panel remains visible even when the incoming layout configures its future placement.
- **GIVEN** the canonical PR Details panel is configured in a non-Agent group, **WHEN** the user explicitly opens a second pull request or merge request, **THEN** the new keyed review tab joins that group; reopening an existing keyed tab focuses it without moving it.
- **GIVEN** a layout without the canonical PR Details panel, **WHEN** the user explicitly opens a specific pull request or merge request, **THEN** the keyed review tab opens in the center fallback group without creating a new split.
- **GIVEN** a default profile without Terminal and a task environment with no saved layout, **WHEN** the user first opens that task, **THEN** the workbench has no Terminal tab and no default user shell is created.
- **GIVEN** an existing task with a task-specific layout and no pending inactive-task changes, **WHEN** the user changes the default profile and returns to that task, **THEN** the task-specific layout and saved active panel are unchanged.
- **GIVEN** an existing desktop task with a saved active panel and a Changes panel outside the Agent group, **WHEN** a meaningful Git or commit update is detected while the task is inactive and the user returns to it, **THEN** the task-specific geometry is restored and Changes becomes the active panel after restoration completes.
- **GIVEN** an existing desktop task with already-known changes and a saved active panel, **WHEN** the user reloads that task without a new Git or commit update, **THEN** the saved active panel remains active.
- **GIVEN** an existing task with a task-specific layout, **WHEN** the user chooses Reset Layout, **THEN** the latest effective default profile replaces that task's layout.
- **GIVEN** two tasks whose active sessions share one task environment and a desktop workbench with Agent in the center and Files or Changes above Terminal on the right, **WHEN** the user switches between those tasks, **THEN** the incoming Agent replaces the outgoing Agent in the same group, the right column remains vertically split, the root remains horizontally split, and the same geometry survives reload.
- **GIVEN** a normal desktop task layout restore completes while the active session's Agent panel is absent, **WHEN** the center group is empty, **THEN** the Agent panel is restored into that group and activated before the workbench is shown; when Plan or another valid saved center tab is active, restoring Agent does not steal focus from it, and a deliberately maximized non-Agent group remains unchanged.
- **GIVEN** a desktop task layout whose Agent group has Chat selected beside an unseen Plan while Files or Changes owns global focus, **WHEN** session reconciliation replaces Chat with the active session's Agent panel, **THEN** Agent remains selected in its group, the globally focused Files or Changes panel remains focused, Plan is never selected or marked seen, and the user is not switched to Plan.
- **GIVEN** a desktop task layout with Agent selected beside PR Details and a different task currently has PR Details selected, **WHEN** the user returns and the saved Agent ID is `chat` or belongs to an earlier session, **THEN** the returning task's live Agent panel is selected and PR Details remains a background tab.
- **GIVEN** a desktop task layout with PR Details deliberately selected beside Agent, **WHEN** the user switches away and returns without pending inactive-task changes, **THEN** PR Details remains selected rather than being rewritten to Agent.
- **GIVEN** a desktop task layout with an Agent selection in one group and Files or Changes selected in another globally focused group, **WHEN** dynamic session-panel reconciliation completes during task restoration, **THEN** both group-local selections are restored and the saved global group remains focused.
- **GIVEN** a task environment whose right column has never been manually resized, **WHEN** its desktop workbench moves from a large monitor to a laptop-sized workbench and back, **THEN** the Files, Changes, and Terminal column follows the default ratio at each width and returns to the large-workbench ratio.
- **GIVEN** a task environment whose right column was manually resized through its desktop sash, **WHEN** the workbench moves between large and laptop-sized displays, **THEN** that requested width is restored whenever it fits and is temporarily clamped only to preserve the current screen's minimum center width.
- **GIVEN** a custom default profile, **WHEN** the user deletes it and confirms, **THEN** the built-in Default becomes effective.
- **GIVEN** a profile draft with Agent removed, duplicate reusable panels, or an empty group, **WHEN** the user attempts to save, **THEN** saving is blocked and the invalid locations are identified.
- **GIVEN** a backend save failure, **WHEN** the user saves a profile edit, **THEN** the draft remains available and the previous persisted layout stays selected.
- **GIVEN** a legacy unreadable saved profile, **WHEN** the Layouts page loads, **THEN** the profile remains available for non-editor management, is marked unavailable for visual editing, and is not silently modified.

## Out of scope

- Customizing mobile or tablet task-detail layouts.
- Auto-activating Changes merely because a task has an already-known non-zero change count.
- Forcing Agent active on every task switch when the task's saved non-Agent selection is valid.
- Changing the global app sidebar width or other layout-profile split proportions.
- Forcing a changed default onto existing task-specific layouts without Reset Layout.
- Configuring task-specific panels such as individual files, diffs, commits, keyed pull-request or merge-request tabs, extra sessions, or extra terminals. The canonical PR Details panel is reusable and remains in scope.
- Mutating the code-defined built-in definitions; direct edits are persisted as hidden user overrides.
- Sharing profiles between users or scoping profiles to a workspace, repository, agent, or executor.
