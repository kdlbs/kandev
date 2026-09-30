---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
---

# Workspace ACP idle suspension design

## Boundary and requirement mapping

`REQ-EXECUTORS-IDLE-PARKING-001` maps to workspace policy, suspension, recovery, and settings below.
The backend decides eligibility from Kandev activity. The runtime stops the exact process and preserves its recovery data.
No OpenCode-specific quiescence endpoint, provider admission fence, or process-descendant probe is required.
The existing agentctl disconnected-instance reaper and its stream guard remain unchanged.

## Workspace policy and persistence

Add typed workspace fields `acp_idle_suspension_enabled` (false) and `acp_idle_timeout_minutes` (120).
Store both through the canonical workspace repository and expose them through existing workspace read/update contracts.
Use pointer fields in partial updates: omitted means unchanged; false is an explicit update; reject null and nonpositive timeout values.
Apply the same validation and defaults on SQLite and PostgreSQL, including create, migration, update, and reset paths.
Authorization follows existing workspace settings access. Tenant/workspace isolation remains mandatory.

This is an ordinary workspace preference, not a deployment release toggle.
It neither reuses nor changes `agentctl.idleTimeout` / `KANDEV_ACP_IDLE_TIMEOUT`.
The policy applies immediately. Enabling or changing the timeout starts a fresh interval for settled candidates, avoiding immediate bulk shutdown.
Disabling invalidates uncommitted suspension claims. It does not wake every suspended session.

Persist an explicit idle-suspension reason bound to the runtime execution and session incarnation.
Extend the durable runtime inventory through a migration; do not infer suspension from absence of a live process.
`executors_running` remains the sole owner of the provider resume token under ADR 0025.
Retain suspension provenance across backend restart so focus can distinguish this policy from a manual stop.

## Idle clock and eligibility

Maintain a monotonic candidate clock keyed by workspace, session, execution, and activity generation.
Settlement or an explicit focus event starts/refreshes the interval. A focus-triggered resume starts a fresh interval after readiness.
A new prompt or known active work invalidates the old interval; settlement starts another interval after work ends.
An unchanged open tab is not a heartbeat. Passive stream traffic and row timestamps are not semantic activity.
After backend recovery without trustworthy idle history, start a full interval after observing a settled live session.

Use the existing orchestrator maintenance owner with a scan cadence at most 30 seconds.
Inspect durable runtime inventory independently of coarse state, then evaluate each row's workspace policy and session eligibility.
Reject active turns, dispatch claims, pending clarification/permission, and known active background work.
A failed read of a Kandev-owned guard skips that candidate. Missing provider internals do not constitute a failed guard.
Existing task-owned LSP leases must transfer or detach according to their owner contract before instance teardown.
Active terminal operations and workspace mutations block teardown while they use the instance; passive subscriptions do not.

## Suspension and concurrency

Introduce a narrow `SuspendIdle` operation through `runtime.Runtime` and its facade.
Use session lifecycle coordination plus the execution's prompt generation to claim the exact candidate.
Message, focus, and workspace-operation admission must invalidate or join that claim rather than race a blind stop.
Recheck workspace policy, current identity, activity generation, and known work at the destructive boundary.
Do not hold locks across I/O that can synchronously publish callbacks acquiring those locks.

Close owned streams and stop the selected ACP instance through the existing executor backend.
Preserve task-owned compute, shared agentctl processes, worktrees, and credentials. Stop only the selected agent process tree.
Executor implementations need an explicit suspension reason if ordinary stop deletes retained resources.
A failed or timed-out stop retains ownership until exact-instance reconciliation establishes the result.

After confirmed stop, conditionally mark the inventory row stopped with its idle-suspension reason.
Preserve the resume token and workspace. Do not remove in-memory ownership until durable repair succeeds or a retry record owns it.
Parking outcomes must bypass ordinary `agent.stopped` cancellation behavior.
Delayed completion/stopped events must match both execution and prompt generation before any state or turn mutation.

## Message and focus recovery

Use one idempotent recovery owner per session/incarnation for all callers.
Route every accepted actionable user, agent, automation, and queued-message delivery through it before dispatch.
Concurrent callers join the same resume; keep existing message identity/deduplication to prevent duplicate delivery.

Extend the task/session activation path used by `use-session-resumption.ts` with an authenticated focus intent.
Send focus only on explicit selection/open or browser visibility/window focus return for the selected task/session.
Debounce duplicate events and check durable idle-suspension provenance on the backend.
Hidden mounted tabs, generic session reads, and list subscriptions cannot wake processes.
The focus exception bypasses prevent-auto-start-on-open only for this idle-suspended session.
Workflow ownership, archive checks, and authorization still apply. Focus must not wake every sibling session.

Resume through the existing provider resume/load path using the saved token. Focus sends no prompt.
Clear suspension provenance only after readiness; failed resume retains it and the existing recovery UI.
If restoration is unsupported or the token is absent before suspension, retain the process and report the precise reason.
Do not silently start a fresh provider conversation.

## Provider and executor coverage

| Boundary | Behavior | Required evidence |
| --- | --- | --- |
| All ACP provider families | Same state/timeout policy, normal provider restore | Table-driven tests with varied provider IDs; no provider allowlist |
| OpenCode | Same policy as other ACP agents | Shared suspend/resume tests; no special upstream API gate |
| Local, SSH, container, Kubernetes, plugin executor | Stop one agent; retain task compute and workspace | Executor contract tests for supported stop/resume paths |
| Unsupported restore or incomplete token | Retain with diagnostic | Explicit capability/token tests |
| Passthrough or taskless run | Existing lifecycle | Exclusion tests |

Do not assume every executor's ordinary StopInstance preserves task-owned resources. Test and adapt its suspension reason explicitly.
Provider process-internal work beyond ACP visibility is a limitation of the opt-in retention policy, not a new eligibility prerequisite.

## Settings and mobile behavior

Add a resource-saving card to Workspace settings Overview using the existing workspace edit surface.
Use a switch, numeric timeout in minutes, concise explanation, and the existing save/error pattern.
Keep the saved timeout visible but disabled when the switch is off. Explain resume on focus or message.
All copy uses localization in six supported languages; generate Traditional Chinese with the repository command.

Desktop places the timeout label and field on one row. Phone stacks them in the same card within the existing page scroll owner.
Use 28px desktop controls and at least 44px phone touch targets, with no nested scroll panel or horizontal overflow.
The existing mobile task navigation and composer remain the recovery surfaces.

## Observability

Log successful suspension/resume with workspace/session/execution identity and reason, never resume tokens.
Use bounded skip reasons for active work, pending input, resource use, missing restore data, stale identity, disabled policy, and teardown failure.
Report a scan summary without repeating unchanged per-instance logs on every tick.

## References

- [Ownership decision](../../../decisions/2026-09-28-attached-idle-runtime-parking.md)
- [Runtime inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Requirements](../requirements/idle-runtime-parking.md)
- [Plan](../../../plans/attached-acp-idle-parking/plan.md)
