---
status: draft
system: tasks
created: 2026-08-19
owners:
  - cfl12
---
# Task Launch Failure Recovery Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001: Task Launch Failure Recovery

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

A task launch can fail after its base branch disappears or its stored default branch becomes stale.
Today, the user sees a transient raw Git error and no durable recovery path.

PR-review automation can also launch work after the relevant PR is complete.
This wastes an agent run and can create a misleading failed task.

## What

- A workflow `on_enter` auto-start does not launch work when every relevant GitHub PR is terminal.
- The gate keeps the task in its current workflow step and records a durable informational reason.
- A manual launch always bypasses the PR gate.
- Every handled launch error has a typed category, a safe message, and an ordered set of recovery actions.
- The task surface shows the error after reload. The launch toast points the user to that surface.
- A stale repository default resolves from the live remote default before the launch stops.
- A successful fallback writes the resolved branch to the exact `task_repositories` row.
- Recovery actions target one task-repository row. Multi-repository and multi-branch tasks stay unambiguous.
- Desktop and mobile provide the same recovery outcomes.

## Data model

### Session-owned launch error

A launch that created a session stores its error in `task_sessions.metadata["last_agent_error"]`.
The existing `LastAgentError` record gains two fields.

```text
LastAgentError
  message             string       safe user-readable message
  occurred_at         timestamp
  agent_execution_id  string       optional
  code                string       stable failure category
  details             string       optional bounded detail
  recovery_actions    []string     ordered known values, maximum 3
  task_repository_id  string       optional exact failed row
  stamp               string       optional bounded identity for typed launch errors
  dismissed_at        timestamp    optional
```

### Task-owned pre-session launch error

An auto-start gate runs before a session exists.
It stores an informational error in `tasks.metadata["last_launch_error"]`.

```text
TaskLaunchError
  message             string       safe user-readable message
  occurred_at         timestamp
  code                string       stable failure category
  details             string       optional bounded safe diagnostic detail
  recovery_actions    []string     ordered known values, maximum 3
  task_repository_id  string       optional exact affected row
  stamp               string       stable identity for idempotent replay
```

The gate derives `stamp` from the sorted relevant PR identities and states.
The gate preserves the record and its timestamp when the same stamp already exists.

The failure categories are:

- `base_branch_missing`
- `pr_already_closed`
- `default_branch_unresolved`
- `generic_launch_failure`

The recovery actions are:

- `retry_default`
- `pick_base_branch`
- `mark_review_done`

### Bounded task projection

`TaskStatusSummary.active_error` projects the newest active task-owned or session-owned error.
It has this wire shape:

```text
active_error
  session_id          string       optional for a task-owned error
  task_repository_id  string       optional exact affected row
  stamp               string
  occurred_at         timestamp
  preview             string       maximum 512 UTF-8 bytes
  category            string       maximum 64 UTF-8 bytes
  recovery_actions    []string     known values only, deduplicated, maximum 3
```

The projector ignores unknown recovery actions.
It preserves the declared action order after it removes duplicates.
Malformed metadata cannot make the full summary invalid.

GitHub PR state remains in `github_task_prs`.
Task-repository PR identity remains in `task_repositories.metadata["pr_number"]`.
This feature adds no database column or table.

## API surface

### WebSocket action `task.launch.recover`

The new action accepts this payload:

```json
{
  "task_id": "task-id",
  "session_id": "optional-failed-session-id",
  "task_repository_id": "required-for-branch-actions",
  "action": "retry_default | pick_base_branch | mark_review_done",
  "base_branch": "required-only-for-pick_base_branch",
  "error_stamp": "required-current-active-error-stamp"
}
```

The success response is:

```json
{
  "ok": true,
  "task_id": "task-id",
  "session_id": "optional-new-or-reused-session-id"
}
```

The handler authorizes `task_id` first.
If `session_id` exists, the handler proves that the session belongs to the task.
If `task_repository_id` exists, the handler proves that the row belongs to the task.
The handler rejects a request when `error_stamp` does not match the current active error.

`retry_default` and `pick_base_branch` require `task_repository_id`.
`mark_review_done` does not require a session or repository row.
The existing `session.recover` action and its action values do not change.

### Status delivery

Boot state, task reads, and `task.status_summary.updated` carry the extended `active_error`.
The contract remains a complete replacement value with a monotonic revision.

### Default-branch resolution

`gitref.DefaultBranchOrEmpty` returns empty when only the current local `HEAD` branch exists.
It stays a pure local-ref helper and does not run a network command.

The worktree manager owns live remote-default refresh through
`Manager.ResolveRemoteDefaultBranch(ctx, repoPath)`.
It first reads `refs/remotes/origin/HEAD`.
If necessary, it runs one bounded, noninteractive `git remote set-head origin --auto` command.

## State machine

### Relevant PR selection

The auto-start gate compares each `task_repositories` row with active PR links for the task.

1. A positive metadata `pr_number` matches the exact `(repository_id, pr_number)` PR.
2. Without that identity, an exact normalized `(repository_id, checkout_branch, head_branch)` match applies.
3. A PR for a different repository or checkout branch is not relevant.

Branch normalization trims whitespace and one leading Git ref prefix.
The accepted prefixes are `refs/heads/`, `refs/remotes/origin/`, and `origin/`.
The remaining branch comparison is case-sensitive.

The gate skips launch only when at least one relevant PR exists and every relevant PR is `merged` or `closed`.
An `open`, empty, or unknown state keeps the normal launch path.
This rule makes an open relevant PR win over a terminal sibling PR.

### Launch transitions

| Trigger | Condition | Outcome |
| --- | --- | --- |
| Workflow `on_enter` | All relevant PRs are terminal | No session or worktree starts. The task keeps its step and stores `pr_already_closed`. |
| Workflow `on_enter` | A relevant PR is open or unknown | The launch proceeds. |
| Workflow `on_enter` | No relevant PR exists or lookup fails | The launch proceeds. |
| Manual launch | Any PR state | The launch proceeds. |
| Launch preparation | The selected base and all fallbacks are missing | The session and task enter their existing failed states. The session stores `base_branch_missing`. |
| Launch preparation | A valid fallback resolves | The launch continues. The exact task-repository base branch self-heals. |

A successful later launch clears `tasks.metadata["last_launch_error"]`.
A failed manual launch replaces the projected error only when its occurrence time is newer.

### Recovery transitions

- `retry_default` resolves the live remote default for the targeted row.
  It writes the repository default and the task-repository base, then relaunches.
- `pick_base_branch` validates the selected remote branch for the targeted row.
  It writes the task-repository base, then relaunches.
- `mark_review_done` lists the workflow steps in position order.
  It selects the final step only when `workflow/models.IsTerminalStep(final, nil)` returns true.
  It also requires the relevant PR selection to contain only terminal states.
  It moves the task through the normal task move service, including history, WIP, state, and events.
  A recovery-only move option permits `FAILED` to become `COMPLETED` at that terminal step.
  Normal manual moves continue to preserve failed and cancelled states.

The UI offers `mark_review_done` only when the workflow and PR conditions are valid.
A forged request fails when either condition is invalid.
The action is idempotent when the task already occupies that terminal step.

## Failure modes

- If the PR lookup fails, the gate logs the error and launches normally.
- If no PR link matches a task-repository row, the gate launches normally.
- If relevant PR rows contain conflicting states, any open or unknown state launches normally.
- If live remote-default refresh times out, the error stays distinct from an unresolved default.
- If live remote-default refresh has an auth or network error, the typed cause remains available for logs.
- If no remote default resolves, the UI shows `default_branch_unresolved` and offers `pick_base_branch`.
- If branch existence cannot be determined, the error remains a verification error, not a missing-branch error.
- If task-repository self-heal fails, the launch continues and logs a warning.
- If repository identity is empty or ambiguous, the backend omits repository-scoped recovery actions.
- If the workflow has no valid terminal step, the backend omits `mark_review_done`.
- If a relevant PR is open or unknown, the backend omits `mark_review_done`.
- If a recovery request names a foreign session or repository row, the request fails without mutation.
- If a recovery request names a stale error stamp, the request fails without mutation.

The typed task error replaces the old missing-branch warning message and its archive/delete actions.
The old `missing_pr_branch_recovery_claimed` metadata claim no longer applies to handled launch errors.
The handled path does not suppress the new pointer toast.

## Persistence guarantees

Session-owned errors survive restart in `task_sessions.metadata`.
Pre-session gate outcomes survive restart in `tasks.metadata`.
The status projection rebuild reads both sources and selects the newest active error.

A successful recovery clears the source error after the required write and relaunch or move succeeds.
A failed recovery updates the same source with the new category and available actions.
No successful branch write is rolled back after a later launch error.

The corrected repository default persists in `repositories.default_branch`.
The corrected task base persists in the exact `task_repositories.base_branch` row.

## Desktop and mobile behavior

The task Chat surface shows one persistent launch-error card before the empty-state message.
The card uses the same projection and action handler on all viewports.
If a matching failed session renders the card, the summary path does not render a duplicate.

Desktop shows recovery buttons inline and opens the existing branch picker pattern.
Phone layouts keep the card inline and wrap actions without horizontal page overflow.
The branch choice uses `MobilePickerSheet`, with the task Chat surface as its entry point.

Each new recovery control has a touch target of at least 44px.
The task Chat surface remains the only vertical scroll owner.
The picker owns its internal list scroll and clears the bottom safe area.

## Scenarios

- **GIVEN** a task row linked to one merged PR, **WHEN** `on_enter` auto-start runs, **THEN** no session starts and the task shows `pr_already_closed` after reload.
- **GIVEN** a task with an open PR and a terminal PR, **WHEN** auto-start runs, **THEN** the launch proceeds for the open PR.
- **GIVEN** one repository with two task branches, **WHEN** only the sibling branch PR is terminal, **THEN** the current branch launch proceeds.
- **GIVEN** a PR lookup error or no relevant PR, **WHEN** auto-start runs, **THEN** the launch proceeds.
- **GIVEN** absent selected and fallback bases, **WHEN** the live default resolves, **THEN** launch continues and the exact row self-heals.
- **GIVEN** no usable base or default, **WHEN** launch preparation stops, **THEN** the failed session shows `base_branch_missing` with unambiguous actions.
- **GIVEN** one multi-repository row with a missing base, **WHEN** the user selects another base, **THEN** only that row changes before relaunch.
- **GIVEN** a valid terminal step, **WHEN** the user selects Mark review done, **THEN** the move completes the task without launching.
- **GIVEN** a reopened relevant PR, **WHEN** the user sends a stale Mark review done action, **THEN** no task mutation occurs.
- **GIVEN** no valid terminal workflow step, **WHEN** the task error renders, **THEN** Mark review done is absent.
- **GIVEN** a local import with only a feature-branch `HEAD`, **WHEN** default detection runs, **THEN** the stored default stays empty.
- **GIVEN** a handled launch error, **WHEN** the browser receives it, **THEN** the toast points to task details without raw Git output.
- **GIVEN** recovery on a phone, **WHEN** the user chooses a base, **THEN** the task relaunches without horizontal page overflow.

## Out of scope

- A background poller for all repository defaults.
- A bulk repair of existing repository-default values.
- A PR gate for manual launches.
- PR gating for providers other than GitHub.
- Changes to ACP resume semantics.
- A different self-heal target for stacked PRs.