---
status: current
system: agents
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
---

# Claude Monitor lifecycle evidence

## Ownership

The ACP adapter owns Monitor registration, identity, and termination evidence.
The [shared workload projection](background-work.md) owns persistence and
publication. Desktop and phone use that projection for the chip and History.

| Requirement | Design boundary |
| --- | --- |
| REQ-AGENTS-BACKGROUND-WORK-002 | Workload identity, prompt ownership, and late delivery |
| REQ-AGENTS-BACKGROUND-WORK-005 | Shared running count, History, and draft |
| REQ-AGENTS-BACKGROUND-WORK-010 | Active Monitor visibility and matched termination |

## Evidence

Claude ACP v1 `0.107.0` and `0.109.0` report Monitor registrations as tool
results. The installed bridge routes structured task endings through
`AsyncTaskRuntime` when the client advertises `asyncTasks`. Kandev connects
without that capability. The separate `terminal_exit` mechanism tracks detached
Bash terminals. The [async-task decision](../../../decisions/2026-11-01-claude-async-task-lifecycle-capability.md)
records the wire decoding and turn behavior required to revisit that capability.

The adapter can consume two forms of explicit ACP wire evidence on the owning
session:

1. **Direct Monitor terminal tool update:** a terminal `tool_call_update` for a
   known Monitor requiring a terminal status, the matching tracked launch tool
   ID, and the owning ACP session. Treat the registration banner as a launch.
   Accept a matching close after handoff using the original binding. Map the
   provider outcome to the existing workload states; a close with an
   unavailable outcome maps to `ended`.
2. **Completed `TaskStop` tool update:** when Claude explicitly stops a running
   Monitor via `TaskStop`, the bridge emits no follow-up update on the original
   Monitor tool-call ID and Claude writes no native `<task-notification>` for
   the stopped Monitor. Require the Claude dialect, `_meta.claudeCode.toolName`
   equal to `TaskStop`, terminal `completed` status, the owning ACP session,
   and a confirmed `task_id` (from `_meta.claudeCode.toolResponse.task_id` or
   the completed JSON `rawOutput.task_id`, matching `rawInput.task_id` when
   present) that resolves to a tracked Monitor in `activeMonitors[sessionID]`.
   Settle only that tracked Monitor as `interrupted`, emit both its
   `EventTypeBackgroundWorkUpdated` observation and its terminal Monitor
   `tool_update`, and record completion so duplicate or late evidence stays
   idempotent. Failed, non-terminal, foreign-provider, or unmatched `TaskStop`
   updates leave all tracked Monitors open.

## Managed ACP v1 completion source

The [Monitor evidence decision](../../../decisions/2026-11-20-claude-monitor-completion-evidence.md)
proposes reading Claude Code's exact-session task notifications on the executor
that launched the bridge. Qualify official bridge releases `0.107.0` and
`0.109.0` and native record versions `2.1.323` and `2.1.328` with captured
sanitized fixtures that leave one Monitor alive after an ACP prompt resolves
and report a separate terminal record for its sibling. Keep a managed-default
regression against `agents.DefaultManagedNPMRuntimeVersion` so bridge upgrades
cannot silently disable Monitor completion evidence.

The source is the native session's JSONL file under the effective Claude
configuration root. Bind the canonical launch directory, native session ID,
and live execution at admission. Use contained, nonblocking file opens and
verify the opened descriptor's identity before reading. Accept qualified
`attachment` records whose attachment type is `queued_command`, origin is
`task-notification` from `session-task`, and prompt contains one terminal task
notification. Also accept the equivalent qualified `user` record. Require the
exact provider task ID and launch tool-call ID. Map provider `stopped` to
Kandev `interrupted`. Use the completion status and timestamp only. Treat
assistant text, output paths, and user messages as ordinary data.

Establish the file position at prompt admission so resumed history stays
outside the read window. Require the Background Work feature flag. Keep the
cursor without polling record bodies until an active Monitor or Monitor launch
needs evidence. Use one cancellable reader per live ACP session while it
tracks eligible Monitors. Read complete records appended within the execution
and use a bounded window. Discard oversized records through their newline and
continue parsing later records. Cover a terminal record that arrives before
its ACP registration is applied. Preserve the read position across turns and
release the reader on session close or execution replacement.
Binding a newly loaded or created ACP session resets the reader. Resuming the
same ACP session rebinds its native generation atomically while preserving its
file position. Duplicate records are idempotent. Incomplete, unavailable, and
unqualified sources leave the last observed state intact. Recovery uses the
existing Unknown and verified owner-exit rules when runtime ownership changes.

## Prompt ownership and projection

Keep the Monitor bound to its original ACP session, launch tool call, and turn.
Foreground idle, same-prompt resolution, successor handoff, and browser reload
preserve the active state of an attached Monitor. The prompt-end sweep leaves
qualified Monitor bindings to their workload lifecycle. Prompt ownership
continues to follow [ADR 0049](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md).

Publish every native completion through `EventTypeBackgroundWorkUpdated` and
the existing typed `streams.WorkloadRunObservation`. Commit one observation
per accepted transition even when transcript updates are suppressed. Preserve
workload ID, source call, origin turn, and capabilities. When building the
initial registration observation from a normalized Monitor tool update that
carries no `ToolTitle` or `ToolName`, populate `Title` from the Monitor's
`description` or `command` (falling back to `Monitor`) so the running summary
identifies the watch before completion. Remove only the finished binding from
runtime tracking. Terminal state stays monotonic across late progress and
repeated completion. Session incarnation and execution identity fence events
from a previous owner.

## Desktop and phone

Running Monitors contribute to the existing chip above the composer. The
running summary shows the Monitor kind and opens its details. The active
Monitor remains visible while the user submits another prompt or reloads the
page. Finished Monitors are accessible in History. Desktop uses the existing
Background Work panel. Phone uses the Panels entry and full-height surface.
Workload updates preserve the composer draft and open detail. The transcript
Monitor card reads the workload projection by its session and source call ID,
so only the matching card stops showing a watching state. This projection
survives reload and does not depend on a foreground tool update.

## Verification

The [repair plan](../../../plans/claude-background-job-settlement/plan.md) covers
a live watch after either an ordinary prompt response or a foreground-idle
handoff. On desktop and phone, reload with both watches live, finish one, and
confirm its sibling remains in the chip. Finish the last watch and verify
History and the draft. Exercise duplicate, early, partial, and stale source
records plus verified owner termination.

The [TaskStop and managed-runtime repair plan](../../../plans/claude-monitor-taskstop-settlement/plan.md)
adds `TaskStop` wire settlement (including failed, unmatched, and sibling-live
cases), managed `0.109.0` / `2.1.328` fixture qualification with a
managed-default version guard, and initial Monitor title projection.
