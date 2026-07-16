---
status: shipped
created: 2026-07-14
owner: kandev
supersedes: docs/specs/workflow-settings-autosave/spec.md
---

# Settings Manual Save

## Why

Settings pages currently mix immediate persistence, section-level Save buttons, and page-level Save buttons. This makes it unclear when a change becomes durable, creates unnecessary intermediate writes while a user is still configuring a resource, and leaves required actions out of view on long pages.

## What

- Editing a persistent configuration value under `/settings` changes a local draft and does not call a persistence API until the user explicitly saves.
- A subtle fixed action appears at the bottom-right of the viewport whenever the current settings route has at least one dirty draft. It remains reachable while the page scrolls and saves every dirty contributor on that route.
- The action reports `Save changes`, `Saving...`, `Saved`, or `Couldn't save`. It prevents duplicate submissions, remains visible after a failure, and retries contributors that are still dirty.
- A successful contributor becomes clean only for the exact draft revision that was submitted. Edits made while a save is in flight remain dirty and require another Save.
- When several cards or sections are dirty, one Save attempts all of them in a stable order. Successful contributors become clean even if another contributor fails; failed and newly edited contributors remain dirty.
- Existing page-level and section-level Save buttons move to the shared floating action when their fields belong to the route. Dialog and sheet forms keep their own explicit Create or Save button.
- A draftable reset-to-default action, including keyboard shortcut, feature-toggle, query, and preset resets, updates the draft and requires Save. It is not treated as an immediate command.
- A visual preview may react immediately when that is the purpose of the setting, such as changing the color theme, but the durable value is not updated until Save. Reloading or discarding before Save restores the saved value.
- Controls remain interactive while dirty. Persistence latency begins only after Save, so experimenting with toggles and selectors does not block further editing.
- Each draftable control whose value differs from its saved baseline uses the shared success-green border/ring so the user can identify the unsaved fields. Its nearest owning card or framed settings group uses the same marker whenever any descendant is dirty. The markers clear when the value returns to its baseline, is discarded, or saves successfully; settings dirty state never uses warning-yellow styling.
- On settings routes, the floating action uses the success-green treatment and does not overlap the Configuration Chat trigger or its open popover.

### Workflow settings

- Confirming Add Workflow creates a local new-workflow draft. The workflow and its initial steps are not persisted until the floating Save action is pressed.
- Workflow metadata, default profile, step fields, transitions, step ordering, workflow ordering, and newly added steps remain local drafts until Save.
- The workflow page may contain multiple dirty workflows. The single floating action saves all dirty workflow contributors.
- A dirty or newly added workflow marks its workflow card and each changed metadata control. A dirty or newly added step marks its pipeline node, selected step configuration panel, and changed controls, while the owning workflow card remains marked.
- New workflow and step drafts use client-only identities until persistence succeeds. References between draft steps are remapped to server identities during Save.
- Removing a step that exists only in the draft is local. Deleting an already persisted step or workflow remains an immediate, explicitly confirmed destructive command; if it has unsaved edits, the confirmation states that those edits will be discarded.
- Import, export, task migration, and confirmed workflow or step deletion remain explicit immediate commands. An imported workflow is a clean persisted baseline.

### Settings-wide coverage

The manual-save rule applies to route-level persistent configuration, including:

- workspace details, repositories, workflows, and automation editors;
- agent, agent-profile, executor, executor-profile, and SSH login-shell configuration;
- appearance, resource metrics, keyboard shortcuts, terminal and shell, editors, notifications, voice mode, prompts, secrets, and changelog-notification configuration;
- utility-agent selection/enabled state and the config-chat profile;
- runtime feature-toggle overrides;
- integration configuration forms, GitHub repository scope/action presets/default queries, and GitHub/Jira/Linear/Sentry watcher enabled state.

### Immediate actions

The following remain immediate because the user invokes a named operation rather than edits a draft field:

- browser permission prompts, connection tests, refreshes, rescans, probes, reveals, copies, imports, exports, and downloads;
- Connect, Disconnect, Clear credential, Remove credential, and dialog-level Create/Save/Delete submissions;
- confirmed destructive actions, including deleting a persisted workflow or step, provider, watch, repository, profile, secret, or other resource;
- task migration, manual watcher runs/resets that alter matched task state, cleanup operations, backups, restores, database maintenance, updates, restart, and factory reset;
- clearing the changelog last-seen marker.

Changing a configuration value to its default is not an immediate action merely because its control is labeled Reset.

## State Machine

Each registered contributor has these observable states:

| State   | Trigger                                                             | Result                                                                                         |
| ------- | ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Clean   | Draft equals its saved baseline                                     | No floating action is shown for that contributor.                                              |
| Dirty   | A draftable value changes                                           | The floating action appears; no persistence request is made.                                   |
| Invalid | A dirty draft fails local validation                                | The action remains visible but Save is disabled with the invalid field identified on the page. |
| Saving  | The user presses Save                                               | The submitted revision is persisted once; duplicate Save is disabled.                          |
| Clean   | The submitted revision succeeds and is still current                | The baseline advances and the contributor leaves the dirty set.                                |
| Dirty   | The submitted revision succeeds but the user edited again in flight | The new revision stays dirty.                                                                  |
| Error   | Persistence fails                                                   | The draft remains visible and dirty; the action reports failure and can be retried.            |

Route navigation with dirty contributors opens an in-app confirmation with `Save and leave`, `Discard and leave`, and `Continue editing`. Browser reload, tab close, and external navigation use the native `beforeunload` warning. A failed `Save and leave` keeps the user on the page. Browser history navigation must not silently discard drafts.

## Failure Modes

- A failed save never replaces the draft with the last server response and never marks that contributor clean.
- Saving multiple contributors is not atomic. The coordinator continues attempting the remaining dirty contributors, reports a route-level error if any fail, and retains only failed or subsequently edited contributors as dirty.
- If a server or websocket refresh arrives while a contributor is clean, it may replace the baseline and draft. If it arrives while dirty, it must not overwrite the user's draft.
- A partially created workflow stays represented by the user's draft. Save retries only missing or still-dirty operations and must not create a duplicate workflow.
- Existing API authorization and validation errors are surfaced beside the shared action and by the relevant field or section when available.

## Persistence Guarantees

- Only a successful explicit Save or an immediate named command changes durable settings.
- Unsaved drafts are route-local and do not survive reload, process restart, or an explicit discard.
- Existing backend persistence contracts remain the source of truth after reload. This feature does not add client-side draft storage.
- Manual saving reduces intermediate writes but does not add optimistic concurrency control. Existing server conflict behavior remains in force.

## Scenarios

- **GIVEN** a saved workflow step, **WHEN** the user toggles Auto-start agent several times, **THEN** the control responds immediately, no update request is sent, and the floating action remains visible until Save.
- **GIVEN** dirty workflow metadata and two dirty workflow steps, **WHEN** the user presses Save, **THEN** all three drafts are persisted and a reload shows the saved values.
- **GIVEN** a new workflow draft, **WHEN** the user reloads before saving, **THEN** no workflow was created and the draft is discarded.
- **GIVEN** two dirty settings contributors and one save fails, **WHEN** Save completes, **THEN** the successful contributor is clean, the failed contributor remains dirty, and the action reports `Couldn't save`.
- **GIVEN** a save is in flight, **WHEN** the user changes another field, **THEN** the completed request does not clear the newer dirty revision.
- **GIVEN** a dirty keyboard shortcut, **WHEN** the user presses Reset, **THEN** the default shortcut is shown locally and no persistence request occurs until Save.
- **GIVEN** a dirty feature-toggle override, **WHEN** the user presses Reset to default, **THEN** the inherited value is staged and the runtime flag does not change until Save succeeds.
- **GIVEN** a dirty color-theme draft, **WHEN** the user previews another theme and leaves with Discard, **THEN** the previously saved theme is restored.
- **GIVEN** one or more changed settings fields, **WHEN** their draft values differ from the saved baseline, **THEN** only those controls have the green dirty-field border until they are reverted, discarded, or saved.
- **GIVEN** one or more changed settings fields inside a card or framed settings group, **WHEN** any descendant differs from its saved baseline, **THEN** the changed controls and their owning card use the same green dirty marker and no settings dirty marker uses yellow.
- **GIVEN** a new workflow or workflow step draft, **WHEN** it has not been saved yet, **THEN** the new item, its owning workflow card, and its changed controls are marked dirty until Save succeeds.
- **GIVEN** a dirty settings route, **WHEN** the user navigates elsewhere, **THEN** an in-app confirmation offers Save and leave, Discard and leave, or Continue editing.
- **GIVEN** a persisted workflow step with tasks, **WHEN** the user confirms migration and deletion, **THEN** the destructive operation runs immediately without waiting for the floating Save action.
- **GIVEN** a long settings page on desktop or a 390px mobile viewport, **WHEN** any field becomes dirty, **THEN** the floating action is fully visible, keyboard/touch reachable, clear of safe-area insets, and does not cover the last editable control.
- **GIVEN** Configuration Chat is closed or open on a dirty settings route, **WHEN** the floating action is shown, **THEN** the Save action does not intersect the chat trigger or popover.
- **GIVEN** a clean settings route, **WHEN** it is displayed, **THEN** no floating save action occupies the viewport.

## Out of Scope

- Atomic transactions across unrelated settings endpoints or across multiple workflows.
- Record versioning, merge UI, or multi-user conflict resolution.
- Persisting unfinished drafts across reloads or devices.
- Replacing explicit operation buttons for tests, lifecycle commands, destructive confirmations, or dialog forms.
- Changing backend settings schemas or HTTP contracts solely to support the floating action.

## Implementation Plan

See [the implementation plan](../../plans/settings-manual-save/plan.md).
