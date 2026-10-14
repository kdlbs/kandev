---
status: active
system: office
created: 2026-05-21
owners:
  - jcfs
---
# Automations in Settings Requirements

## Overview

Users want to schedule an agent to run a prompt on a cron (or on a GitHub PR event, or on a webhook) without first navigating to per-workspace settings, picking the right workspace, then drilling down into a workflow. What they get back is informational — a report to read — so it must not land on the board as if it were work someone has to move.

## Requirements

### REQ-OFFICE-AUTOMATIONS-SETTINGS-001: Automations in Settings

**Intent:** Users want to schedule an agent to run a prompt on a cron (or on a GitHub PR event, or on a webhook) without first navigating to per-workspace settings, picking the right workspace, then drilling down into a workflow. What they get back is informational — a report to read — so it must not land on the board as if it were work someone has to move.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.1:** Every automation produces the same kind of run. There is no execution-mode choice — the task/run selector is removed, and the field is ignored. A control that decided both *where a run appeared* and *how it lived* could not be labelled honestly: "Run" silently meant the worktree was destroyed, "Task" silently meant the schedule jammed after one firing.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.2:** Every automation has an optional `repository_ids` field — an ordered list of repository IDs. When non-empty, scheduled and webhook firings pin the task to every listed repository (each on its own default branch, mirroring how the task-creation dialog resolves an explicit repository). When empty, falls back to the workspace's first repository (legacy behavior). `github_pr` triggers always use the PR's own repository and ignore `repository_ids`.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.3:** The editor's repository picker matches the task-creation dialog's UX: lists both registered workspace repositories AND filesystem-discovered repositories under the workspace's roots. Picking a discovered repo registers it with the workspace at automation-save time (one round-trip via `createRepositoryAction`), then stores the resulting id on the automation. After the first save, the selection is promoted from `discovered` to `registered` so subsequent edits don't try to re-register.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.4:** **Multi-repository selection is gated on the selected executor profile's capability**, reusing the task-creation dialog's `getMultiRepoExecutorDisabledReason` predicate (`apps/web/components/task-create-dialog-multi-repo-guard.ts`): `worktree`, `local_docker`, `ssh`, and `sprites` executor types support sibling repositories; `local`, `local_pc`, and `remote_docker` do not.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.5:** When the selected executor profile's type supports multi-repo, the picker renders as a repeatable list (0..N rows) with an "Add repository" control. Each row independently picks a registered or discovered repository; a repository already selected in another row is marked and not independently selectable a second time (mirrors the task-creation dialog's "Already added" marker).
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.6:** When the selected executor profile's type does not support multi-repo, the picker renders as today's single dropdown (registered/discovered repos, or "Auto").
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.7:** The Executor Profile picker disables executor profiles that don't support multi-repo whenever two or more repositories are currently selected, with the same disabled-reason text as the task-creation dialog, so a user cannot silently strand a multi-repository automation on an incompatible executor.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.8:** Automations created via the WS API directly (bypassing the editor) may still combine an incompatible executor with multiple repository IDs; this is a client-side authoring guard, not a backend rejection. Task launch on an incompatible executor fails the same way manual multi-repo task creation would.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.9:** When a user initiates automation deletion from the workspace automation list or the automation editor, the settings UI shall show a confirmation dialog that identifies the automation and states that the deletion cannot be undone.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.10:** When the user dismisses or cancels the deletion confirmation, the automation shall remain and no delete request shall be made. When the user confirms, the UI shall perform one deletion and preserve the existing result for that entry point: remove the list item or return the editor to the workspace automation list.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.11:** On a phone viewport, the deletion confirmation shall remain inside the viewport and expose reachable Cancel and Delete actions with touch-sized hit areas, while preserving the same deletion outcome as the desktop flow.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.12:** Every workspace automation settings list shall expose only rows belonging to its requested workspace, on desktop and phone. While another workspace's list is pending, navigating back to an already loaded workspace shall restore that workspace's rows immediately; settlement of the other workspace's request shall not replace them. With no requested workspace, the list shall expose no rows and no loading state.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.13:** Within one application state and one workspace, the most recently started list request shall own subsequent list publication across all settings consumers. Earlier success or failure shall not replace its result or clear its loading state, in either settlement order. An accepted empty result shall replace previously displayed rows.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.14:** Settings consumers sharing an application state and workspace shall share accepted rows and list loading state. Reopening an already loaded list shall reuse its accepted rows without an automatic request; concurrent initial consumers shall share a pending initial request. Consumers of different workspaces or independent application states shall remain isolated. Removing an initiating consumer shall not strand a remaining consumer's pending list or prevent later cache reuse.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.15:** A current initial list failure shall settle to an empty, loaded, non-loading list. A current refresh failure shall retain that workspace's accepted rows and settle its loading state. An explicit refresh shall recover after either failure. Obsolete failures shall have no effect on accepted rows or current loading state.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.16:** Creating an automation with initial scheduled triggers, adding a scheduled trigger, or updating its supplied schedule config shall reject a non-empty schedule when the scheduler cannot compute a next occurrence within its supported search horizon, including impossible calendar dates such as February 31. Rejection shall persist no new automation or trigger rows, partial initial trigger set, or changes to an existing trigger. An empty schedule shall remain an accepted "not scheduled yet" configuration.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.17:** An already-persisted scheduled trigger with no next occurrence shall never be automatically due. Repeated scheduler checks shall create no runs, skipped runs, or automation-triggered events for that trigger and shall leave its config, enabled state, and evaluation cursor unchanged. Other schedulable triggers shall continue to be evaluated independently.
- **AC-OFFICE-AUTOMATIONS-SETTINGS-001.18:** Schedule validation and automatic due evaluation shall preserve supported five-field expressions, descriptors, intervals, configured timezones (UTC when empty), and daylight-saving behavior. Valid leap-day schedules within the supported search horizon and day-of-month/day-of-week OR expressions such as `0 0 31 2 MON` shall remain eligible. A valid occurrence shall be due at its computed instant and shall not be due before it.

## List contract exclusions

These criteria govern settings list reads and their visible state. They do not
establish coherence between list responses and concurrent mutations, backend
events, other browser tabs, or the separately fetched sidebar and automation-run
lists. Automation mutation results and one-time webhook-secret handling retain
their existing contracts.

## System design

The migrated technical source is split into [part 1](../system-design/automations-settings-01.md), [part 2](../system-design/automations-settings-02.md).
