---
status: draft
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

## System design

The migrated technical source is split into [part 1](../system-design/automations-settings-01.md), [part 2](../system-design/automations-settings-02.md).
