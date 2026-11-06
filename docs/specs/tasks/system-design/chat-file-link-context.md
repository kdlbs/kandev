---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
created: 2026-11-05
owners:
  - kandev
---

# Chat file-link context

## Ownership and mapping

Tasks owns checkout continuity when attachment changes a task workspace.
The [attachment design](attach-workspace-sources.md) retains materialization and lifecycle authority.
This design extends its chat-link contract for criteria `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5` through `.8`.
UI retains the existing file viewers. Workspace file services retain filesystem containment.

| Criteria | Design sections |
| --- | --- |
| 001.5, 001.6, 001.7 | Authoritative roots, Read projection, Live delivery |
| 001.7, 001.8 | Link resolution, Responsive behavior, Verification |

The [root separation decision](../../../decisions/2026-11-05-chat-file-link-workspace-context.md) records the alternatives.
The [delivery package](../../../plans/live-repository-attachment/plan.md) covers issues #5070 and #5071.
Git refresh health follows the independent [Platform design](../../platform/system-design/git-repository-scope-lifecycle.md).

## Authoritative roots

Three paths have separate meanings:

- `workspace_path` identifies the current Files root from the task environment.
- `worktree_path` identifies the primary checkout. It does not establish the agent working directory.
- `agent_workspace_path` identifies the confirmed native-session working directory for relative chat links.

A legacy live attachment changes the first path without changing the third path.
A session that starts at a parent workspace uses that parent as its relative base.
This distinction also applies when the same repository has multiple attached branches.

`AgentExecution.WorkspacePath` supplies the live value.
The current `HarnessSessionGeneration.CurrentWorkspace` supplies the durable value after reload.
Both values refer to the rendered session and its current incarnation.
The mutable raw `task_sessions.workspace_path` column is not durable evidence of the native working directory.
Ordinary session updates can overwrite that column with the effective environment root.

## Read projection

Add a read-only `AgentWorkspacePath` projection to `models.TaskSession`.
Project the latest committed harness generation for the session's current incarnation.
Use `queue_incarnation_id`, with the existing session-ID fallback for legacy incarnations.
Reuse the indexed selection in `GetCurrentHarnessSessionGeneration`.

Extend the common session SELECT and both scan helpers without multiplying session rows.
A correlated subquery can select `current_workspace` through the existing generation index.
Use portable SQL and add no schema migration or per-session application query loop.
An absent generation or empty workspace yields an unknown value.

Expose `agent_workspace_path` in full and summary session DTOs and the model's direct API projection.
An authoritative full projection emits `null` when the value is unknown.
An omitted field in a partial event means that the event carries no update.
The frontend type therefore permits both omission and `null`.
No write path copies this read projection into the environment or harness records.

## Live delivery

Extend `AgentctlEventPayload` and normal ready publication with the launched execution's workspace.
`NotifyWorktreeMaterialized` publishes that same execution value beside the promoted `task_workspace_path`.
It must not substitute the new sibling's `worktree_path`.
Existing execution, attempt, environment, and session fences remain in force.

`handleAgentctlReady` updates the effective root and relative base as separate fields.
The session merge preserves a known base across unrelated partial updates.
An authoritative `null` clears it. Existing request-epoch guards reject stale hydration.
Cover both arrival orders between session hydration and materialization events.

Keep this projection out of cross-task status summaries and public task-row streams.
It belongs to session-detail reads and the existing session-detail event.

## Link resolution

`TaskMarkdownFileLinkProvider` supplies the confirmed base with the effective Files root.
`MarkdownFileLinkContext` passes it through shared and memoized Markdown renderers.
Extend `resolveMarkdownFileTarget` with an optional relative-base input.
Keep the existing absolute-path and source-alias resolution order.

For a relative href, use this precedence:

1. Preserve an explicit current source prefix, such as `master/docs/guide.md` or `secondary/docs/guide.md`.
   Derive prefixes from exact session worktree paths relative to the effective Files root.
   Source registration does not require a host `local_path` on remote executors.
2. Otherwise, resolve the href beneath the confirmed agent workspace.
   Convert the resulting path to a Files-root-relative target.
3. Without a confirmed base, retain the established root-relative behavior for a single known root.
   In an expanded workspace with ambiguous context, return a blocked target.

`docs/guide.md` from `<task>/master` becomes `master/docs/guide.md` after promotion to `<task>`.
A confirmed base equal to `<task>` keeps `docs/guide.md` unchanged.
An explicit `master/` prefix is never added twice.
Current absolute paths, registered source aliases, URL handling, source locations, and supported encoding remain compatible.

Containment uses path components, including existing Windows separator and drive rules.
Traversal, outside roots, ambiguous checkout identity, and unrelated host paths remain blocked.
Never probe repositories for a matching filename or choose the first repository with a match.
The file-open callback still receives one canonical Files-root-relative path.
The agentctl file-content API needs no change.

This contract covers the recorded native working directory.
It does not infer a later shell `cd` or introduce historical per-message working-directory storage.
Native-session relocation and continuation retain their separate lifecycle contract.

## Responsive behavior

Desktop chat opens the existing file panel.
Phone Chat opens `MobileFileViewerPanel` through its existing full-height viewer.
Its Back/close control, safe areas, touch targets, and internal scrolling remain intact.
Shared state and resolution determine the file on both surfaces.
No new control, copy, preference, or breakpoint is necessary.

## Verification

Repository and DTO tests distinguish the environment root, primary checkout, and current harness workspace.
Include a newer incarnation, a parent-root session, absent generation data, and an ordinary session update after promotion.
Resolver and provider tests cover old/new relative links, explicit prefixes, aliases, ambiguity, and duplicate filenames.
The browser scenario performs a real in-turn legacy attachment and repeats the links after reload.
It also covers a parent-root session so a default-primary shortcut cannot pass.
Exact commands and test names belong to the linked work orders.
