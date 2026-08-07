# ADR-2026-07-31-agent-generated-task-titles: Bind Agent Title Generation to Pending Tasks

**Status:** accepted
**Date:** 2026-07-31
**Area:** backend, frontend, protocol

**Refined by:**
[ADR-2026-08-02-single-owner-agent-task-titles](2026-08-02-single-owner-agent-task-titles.md)
for durable first-session ownership and the default-on preference rollout. Where session eligibility or
preference defaults differ, the later decision governs.

## Context

Users can currently create Kanban tasks and subtasks only after choosing a title. Kandev needs an
optional flow where the initial prompt is enough to create the task, a readable provisional title is
available immediately, and the first agent session replaces it with a concise title. The feature
crosses portable user settings, task persistence, first-turn prompt composition, and the task-mode MCP
surface, and it must not cause later sessions to rename an already-titled task.

## Decision

The regular task-creation HTTP contract accepts an explicit `auto_title` opt-in. The task service
derives a provisional title from the first six whitespace-normalized words of the initial prompt and
persists `tasks.metadata.agent_title_pending=true` with the task. The setting that causes the UI to
send this opt-in remains a backend-owned portable user preference; Kandev does not read the current
preference at session launch time.

The executor resolves a title-pending task-mode MCP variant only when the task has
`agent_title_pending=true`; regular task mode does not register `set_task_title_kandev`. The tool is
bound to the MCP server's current task and accepts only the replacement `title`; callers cannot select
another task. Its tool and argument descriptions teach the agent to target about six words in sentence
case. This is generation guidance rather than a new persistence constraint.
First-turn prompt composition includes both the tool inventory entry and the instruction to call it
before other work only while the marker is true, even if the provisional title appears usable. A
successful call replaces the title, removes the marker, and publishes the normal `task.updated` event.
Any ordinary title update also removes the marker so a user rename wins a race with the agent. Calls
made after the marker is gone return a non-mutating `accepted: false` result.

The title-pending task mode is resolved from persisted task intent when the session launches; Config
and Office restrictions still take precedence, and the tool is never registered in Office, Config, or
External mode. Structured sessions receive the conditional instruction in the normal hidden Kandev
context; passthrough sessions receive the equivalent short launch instruction because they have no
hidden system-prompt channel.

## Consequences

- Every created task has a usable persisted title before an agent process starts, even when launch is
  delayed or fails.
- The pending marker, rather than the user's current setting, determines whether a task still needs an
  agent title, so preference changes do not retroactively rename tasks.
- Manual renames and successful agent renames converge on the same normal task-update event path.
- Tasks created without agent naming incur no prompt or tool-schema token cost for this feature.
- Tool discovery remains stable for the lifetime of one MCP server. A successful call removes the
  persisted marker, so later sessions use regular task mode and omit the tool; the originating session
  may retain an idempotent tool until it ends.
- Passthrough users may see the short title instruction in the native TUI because passthrough has no
  separate hidden system channel.

## Alternatives Considered

1. **Reuse `update_task_kandev`.** Rejected because it requires an arbitrary task ID, exposes unrelated
   mutations, and cannot express the one-time pending/no-overwrite contract.
2. **Read the live user setting whenever a session starts.** Rejected because enabling the setting
   would instruct sessions on older manually titled tasks, while disabling it could strand a newly
   created provisional title.
3. **Register the MCP tool for every task-mode session.** Rejected because users who leave the feature
   disabled would pay prompt/tool-schema token cost for a capability their tasks cannot use.
4. **Derive and track titles only in the frontend.** Rejected because creation callers could diverge on
   whitespace, word selection, and pending-state semantics, and the backend could not enforce the
   manual-rename race.
5. **Generate the title with a separate utility-agent request before task creation.** Rejected because
   it adds latency, cost, another failure dependency, and still needs a provisional title.
