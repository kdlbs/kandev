---
status: active
system: tasks
created: 2026-07-31
updated: 2026-08-04
owners:
  - kandev
---
# Agent-Generated Task Titles Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-AGENT-GENERATED-TITLES-001: Agent-Generated Task Titles

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-AGENT-GENERATED-TITLES-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Writing a separate title adds friction when the task prompt already describes the work. Users who
prefer prompt-first creation need tasks and subtasks to appear immediately with a readable label while
letting the assigned agent replace that label with a concise title.

Decisions:
[ADR-2026-07-31-agent-generated-task-titles](../../../decisions/2026-07-31-agent-generated-task-titles.md)
and
[ADR-2026-08-02-single-owner-agent-task-titles](../../../decisions/2026-08-02-single-owner-agent-task-titles.md).

## What

- **Let agents name new tasks** is a per-user setting under **Settings → General → Task Actions**. It is
  enabled by default. New settings records and saved settings that predate the field both resolve to
  enabled; an explicitly saved `false` remains disabled.
- With the setting disabled, the existing task and subtask creation flows remain unchanged: their title
  inputs are visible and the user-supplied title is required. Sessions for those tasks receive neither
  the agent-title prompt instruction nor the `set_task_title_kandev` tool schema.
- With the setting enabled, the **New Task** and **New Subtask** dialogs hide their title inputs. Editing
  or manually renaming an existing task still shows the normal title control.
- Auto-titled creation requires a non-empty prompt. An empty prompt cannot create a task in this mode,
  because Kandev has no source for either a provisional title or the agent's first turn.
- Kandev creates the task with a provisional title before starting or preparing a session. It trims the
  user-visible prompt, splits it on whitespace, and joins its first six words with one ASCII space. A
  shorter prompt uses all available words. No ellipsis is appended and there is no normal
  character-count truncation; the existing absolute 500-character task-title limit remains a safety
  boundary for pathological words.
- The provisional title remains usable if session preparation, agent launch, MCP discovery, or the
  agent's title call fails.
- The first eligible task-mode session whose initial turn begins atomically claims title ownership
  before Kandev records or composes that turn's prompt, configures title-capable MCP mode, or starts
  the agent process. Preparing only the workspace and agentctl without beginning the initial turn does
  not claim ownership and uses ordinary task mode. Config, Office, and External sessions are
  ineligible; structured and passthrough task-mode sessions are eligible.
- Only the owner session is instructed to call `set_task_title_kandev` before any other work or tool
  call, even though the task already has a provisional title. The replacement summarizes the user's
  request with a target of about six words in sentence case and uses a short title phrase rather than a
  sentence or progress update.
- A successful agent title call updates every live task surface through the normal `task.updated`
  event, ends the pending state, and renames each Kandev-generated local branch in the owner session
  from the provisional-title name to a name rendered from the accepted title. Rendering reuses that
  repository's configured worktree branch template and the task's existing branch-name context.
- Branch renaming remains part of the opt-in lifecycle: tasks created without `auto_title: true`,
  including tasks created while **Let agents name new tasks** is disabled, are never renamed by a title
  call. Ordinary human title edits also do not rename branches.
- A task repository with an explicit `checkout_branch` is preserved. This includes tasks created from
  a GitHub pull request or another remote change link, so the session remains on the remote head branch
  and can contribute changes to it. Local-executor sessions are also preserved because Kandev does not
  own their shared checkout branch.
- If the owner session switched away from its recorded generated branch before the title call arrives,
  Kandev preserves the current branch rather than treating that user selection as Kandev-owned.
- In multi-repository tasks, eligibility is evaluated per repository. Kandev can rename generated
  branches and preserve explicit remote checkouts in the same owner session. Sessions other than the
  title owner keep their current branches; future sessions render new branches from the final task
  title through the normal creation path.
- Every other session omits the instruction and tool, including when it launches concurrently with the
  owner or after the owner's launch or title call fails.
- A user or other ordinary task-title update made while the agent title is pending wins: it ends the
  pending state, and a later `set_task_title_kandev` call does not overwrite it.
- Task and subtask dialogs provide the same capability on desktop and phone. Their existing responsive
  surfaces, scroll ownership, repository/profile controls, and primary actions remain unchanged; the
  prompt becomes the first editable field when the title control is absent.

## Data model

`users.settings` stores `agent_generated_task_titles` as a boolean in the existing per-user JSON
settings blob. A missing field is interpreted as `true`; an explicit `false` is preserved.

An opted-in task stores `agent_title_pending: true` in its existing `tasks.metadata` JSON object. The
first eligible initial-turn launch adds `agent_title_owner_session_id: "<session-id>"` with an atomic
compare-and-set that succeeds only while the pending marker is true and no different owner exists.
The claim is idempotent for the same session. Both keys are removed, rather than set to false or an
empty string, when an agent or ordinary title update resolves the title. Tasks without the pending key
are never treated as pending; pending tasks created before the owner key existed are claimed normally
by their first eligible launch.

No database column or schema migration is required.

Existing `task_repositories.checkout_branch` values are the durable signal that a repository was
opened on an explicit branch and must be preserved. Successful generated-branch renames update the
repository-scoped task-environment row and the corresponding running-executor branch snapshots used
by restart and resume. The task workspace directory is not renamed.

## API surface

### User settings

- `GET /api/v1/user/settings`: `settings.agent_generated_task_titles: boolean`
- `PATCH /api/v1/user/settings`: optional `agent_generated_task_titles: boolean`
- `user.settings.updated`: `agent_generated_task_titles: boolean`

### Task creation

`POST /api/v1/tasks` accepts `auto_title?: boolean`.

- When absent or `false`, `title` remains required and existing behavior is unchanged.
- When `true`, `description` must contain non-whitespace text, `title` may be omitted, and Kandev
  derives the persisted provisional title and pending marker from `description`.
- `auto_title` applies only to this creation request. It is not stored as a permanent task preference.

### Task MCP

The owner session for a title-pending task exposes:

```json
{
  "name": "set_task_title_kandev",
  "arguments": {
    "title": "string"
  }
}
```

The tool targets the current task bound to the MCP server. A successful response contains
`{"accepted": true, "task_id": "...", "title": "...", "branch_rename": {...}}`. The
`branch_rename` object reports an aggregate `status` of `renamed`, `preserved`, `partial`, `failed`, or
`not_applicable`, plus repository-scoped `renamed`, `preserved`, and `failed` entries. Preserved entries
identify `remote_checkout`, `local_executor`, or `switched_branch` as their reason. `renamed` means at least one rename
succeeded and none failed, even when other repositories were intentionally preserved; `partial` means
some eligible renames succeeded and others failed; `failed` means every eligible rename failed;
`preserved` means there were branches but none were eligible; and `not_applicable` means the owner had
no branch rows. A task whose pending marker is already gone returns
`{"accepted": false, "reason": "title_not_pending", "task_id": "..."}` without mutation.
An unavailable call from a session other than the persisted owner returns
`{"accepted": false, "reason": "title_not_owner", "task_id": "..."}` without mutation.
Blank titles and titles over the existing 500-character limit are validation errors.

The tool description tells the agent to set a short title phrase for the current task before beginning
work, to target about six words in sentence case rather than a sentence or progress update, and to make
the call even when the existing provisional title appears usable. The `title` argument repeats that
guidance. The word count is generation guidance, not a server-side rejection threshold; the existing
500-character limit remains the hard validation boundary.

`set_task_title_kandev` is registered only for the task-mode MCP server whose bound session matches
`agent_title_owner_session_id` while `agent_title_pending` is true. Ordinary task-mode sessions,
including tasks created while the setting is disabled and non-owner sessions on a pending task, do not
expose its schema. It is also absent from `ModeOffice`, `ModeConfig`, and `ModeExternal`. Registration
is stable for the lifetime of that MCP server; after a successful title call, the owner session may
retain the now-idempotent tool until its server ends.

## Permissions

- Only the task agent connected through the persisted owner session's task-bound MCP server can call
  `set_task_title_kandev` successfully.
- The server injects the current task ID; the agent cannot supply or override it.
- The server also injects its session ID. The title mutation compare-and-set requires that ID to match
  the persisted owner, so catalog gating is not the only ownership check.
- Existing task-service authorization and task-owner scoping apply before the title is read or changed.
- Human title edits keep their existing permissions and take precedence by clearing both lifecycle keys.

## Failure modes

- Saving the setting fails through the existing settings save coordinator; the persisted value and
  creation behavior remain unchanged.
- Auto-titled creation with an empty prompt returns a validation error and creates no task or session.
- If the owner agent never starts, cannot discover MCP, ignores the instruction, or receives an MCP
  error, the provisional title remains. Ownership is not reassigned, and later sessions receive neither
  the instruction nor the tool. The same owner session retains the capability while the title is
  pending if that session is retried or resumed.
- If concurrent initial-turn launches race, exactly one session persists ownership. The loser launches
  without the instruction or tool.
- If ownership persistence fails, Kandev fails that launch before recording or sending its initial
  prompt, configuring title-capable MCP mode, or starting the agent process. The task remains unowned
  and provisionally titled so a later launch attempt can retry the claim without exposing the
  capability ambiguously.
- If a non-owner somehow submits the internal title action, the title remains unchanged and the call
  returns `title_not_owner`.
- If a human changes the title before the agent call arrives, the human title remains and the tool call
  returns `title_not_pending`.
- If the title update cannot be persisted, the tool returns an error, the pending marker remains, and no
  success event is published.
- The title compare-and-set completes before any Git side effect. If one or more branch renames fail,
  the accepted title and cleared pending state are not rolled back. Successful repositories keep their
  new names and durable snapshots; failed repositories keep their old names. The tool reports the
  partial or failed outcome, and a repeated title call does not retry because the title is no longer
  pending.
- A configured branch template can still produce a collision, including when it omits `{suffix}`. The
  underlying Git rename fails normally and is reported without deleting, force-renaming, or changing a
  remote branch.

## Persistence guarantees

The setting survives browser and backend restarts as part of backend-owned portable user settings and
applies across the current user's workspaces. The provisional title and pending marker survive backend,
session, and executor restarts as part of the task row. Once claimed, the owner session ID survives in
the same metadata. MCP catalog state is reconstructed from task mode, pending state, and owner identity;
the same owner can recover its capability, while a different session cannot inherit it after restart.
After a successful generated-branch rename, repository-scoped workspace and executor snapshots use the
new branch name so backend restart, executor recovery, and session resume do not restore the
provisional-title name.

## Scenarios

- **GIVEN** the setting is missing from a new or legacy settings record, **WHEN** settings are loaded,
  **THEN** agent-generated task titles are enabled.
- **GIVEN** the setting was explicitly saved as disabled, **WHEN** a user opens either creation dialog,
  **THEN** the title input is visible and required exactly as before.
- **GIVEN** a task was created while the setting was disabled, **WHEN** its task-mode session starts,
  **THEN** its system context contains no agent-title instruction and its MCP catalog does not contain
  `set_task_title_kandev`.
- **GIVEN** the setting is enabled, **WHEN** a user opens **New Task** or **New Subtask** on desktop or
  phone, **THEN** no title input is rendered and the prompt is the first editable field.
- **GIVEN** the setting is enabled and the prompt contains leading whitespace, line breaks, repeated
  spaces, and more than six words, **WHEN** the task is created, **THEN** its immediate title is the
  first six words joined by single spaces and it is marked title-pending.
- **GIVEN** the setting is enabled and the prompt contains fewer than six words, **WHEN** the task is
  created, **THEN** its immediate title contains every normalized prompt word without an ellipsis.
- **GIVEN** the setting is enabled and the prompt is empty, **WHEN** the user tries to create the task,
  **THEN** creation is blocked with prompt-required guidance and no task is created.
- **GIVEN** a title-pending task has no owner, **WHEN** a structured task-mode session begins its
  initial turn, **THEN** it atomically becomes the owner, is told to call `set_task_title_kandev`
  before any other work, and exposes the tool with the six-word sentence-case guidance.
- **GIVEN** a title-pending task has no owner, **WHEN** a passthrough task-mode session begins its
  initial turn, **THEN** it atomically becomes the owner and the equivalent short instruction precedes
  the user prompt in the native TUI.
- **GIVEN** two eligible sessions begin their initial turns concurrently, **WHEN** their ownership
  claims race, **THEN** exactly one receives the instruction and tool and the other receives neither.
- **GIVEN** the owner launch fails or never sets the title, **WHEN** another task-mode session begins,
  **THEN** the provisional title remains and the later session receives neither the instruction nor
  the tool.
- **GIVEN** the agent follows the tool guidance and calls `set_task_title_kandev` with a valid few-word
  title, **WHEN** persistence succeeds, **THEN** the task title changes, pending state ends, and
  connected task surfaces update.
- **GIVEN** an auto-titled task owner is on a Kandev-generated branch, **WHEN** its title call is
  accepted, **THEN** the branch is renamed from the provisional-title rendering to a rendering of the
  accepted title using that repository's configured template, and restart/resume snapshots record the
  new name.
- **GIVEN** a task was created without `auto_title: true`, **WHEN** its title is edited, **THEN** no
  title-driven branch rename occurs.
- **GIVEN** an auto-titled task started from a GitHub pull request or other direct remote checkout,
  **WHEN** the owner title call is accepted, **THEN** the task title changes but the checked-out remote
  branch name is preserved.
- **GIVEN** an auto-titled Local-executor task, **WHEN** the owner title call is accepted, **THEN** the
  shared checkout branch is preserved.
- **GIVEN** an auto-titled task whose owner switched to another branch before the title call, **WHEN**
  the owner title call is accepted, **THEN** the selected branch is preserved and reported as preserved.
- **GIVEN** an auto-titled multi-repository task mixes a generated branch with a direct remote
  checkout, **WHEN** the owner title call is accepted, **THEN** the generated branch is renamed and the
  direct remote checkout is preserved with both outcomes reported.
- **GIVEN** a generated-branch rename fails after title persistence, **WHEN** the tool returns, **THEN**
  the accepted task title remains, the failed repository stays on its old branch, and the response
  reports the failure without mutating any remote branch.
- **GIVEN** a second task session already exists when the owner sets the title, **WHEN** branch renaming
  runs, **THEN** only the owner session's eligible branches are renamed and the other session keeps its
  current branches.
- **GIVEN** a user manually renames a title-pending task, **WHEN** the agent later calls
  `set_task_title_kandev`, **THEN** the user title is preserved and the tool returns
  `title_not_pending`.
- **GIVEN** an agent title call fails or never occurs, **WHEN** the user returns after a restart,
  **THEN** the provisional title remains visible and the task still has a usable name.
- **GIVEN** any session other than the owner starts before or after title resolution, **WHEN** Kandev
  composes its first turn and MCP catalog, **THEN** both omit `set_task_title_kandev` guidance and
  schema.

## Out of scope

- Changing titles supplied to `create_task_kandev`, Office tasks, automation runs, integration imports,
  plugin-created tasks, or external task-creation clients that do not opt into `auto_title`.
- Generating a title through a separate utility-agent request before task creation.
- Reassigning title ownership or retrying the title instruction with a different session after the
  owner launch or title call fails.
- Removing manual task rename/edit controls.
- Renaming remote branches, deleting the provisional branch remotely, renaming the task workspace
  directory, or renaming branches in non-owner sessions.
- Automatically retrying a failed Git rename after the title-pending lifecycle has resolved.

## Implementation plan

- [Agent-Generated Task Titles](../../../plans/agent-generated-task-titles/plan.md)
- [Agent Title Branch Renaming](../../../plans/agent-title-branch-renaming/plan.md)
