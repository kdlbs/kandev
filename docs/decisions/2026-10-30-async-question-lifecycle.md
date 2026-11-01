# ADR-2026-10-30-async-question-lifecycle: Separate async requests from blocking clarification waits

**Status:** accepted
**Date:** 2026-10-30
**Area:** workflow, protocol, backend, frontend

## Context

[Issue #4836](https://github.com/kdlbs/kandev/issues/4836) asks agents to create questions without ending their current work.
Existing operator questions own a live tool waiter and current conversational turn.
Parent questions persist before an explicit child pause.
Neither lifecycle can express several independent questions that survive ordinary turns.

The [current-turn decision](2026-08-14-current-turn-clarification-ownership.md) prevents stale blocking questions from resuming obsolete work.
Broadening that predicate would weaken an existing safety boundary.
Direct answer prompting would also risk superseding a running turn.

## Decision

Introduce an explicit async request lifecycle owned by Tasks.
The agent chooses it through `async: true` on its existing question tool.
Omitted or false values retain the blocking protocol.
Async records have durable identity, recipient, terminal outcome, and delivery intent independent of blocking clarification messages.

Use the existing exact-session queue and durable replay receipts for continuation delivery.
An accepted answer records an outcome and outbox intent before notification.
Answer arrival never cancels or steers a running turn.
Checkpoint reads can retrieve that same outcome before its queued notification arrives.

Only an explicit dependency wait establishes an async question gate.
Serialize the captured pause with outcome release to prevent a resumed turn from being cancelled by its predecessor.
Request existence alone does not establish a gate.

Forwarding references the child's canonical request and competes for its one terminal outcome.
It preserves direct-parent authorization and the [exclusive question capability](2026-08-08-autopilot-mcp-question-capabilities.md).
An Autopilot parent does not gain operator access through forwarding.

The task chat is the primary operator surface, and it does not depend on the Needs-you Inbox flag.
The Inbox is an optional cross-task view of the same records.
This keeps the lifecycle usable where the Inbox is off, which is the shipped production default.

The agent chooses the mode, guided by a stated rule, because the backend cannot judge whether other useful work remains.

## Consequences

Async questions survive ordinary turns without reviving blocking tool requests.
Operator surfaces must distinguish actionable questions from agents that are actually paused.
With the Inbox off, there is no single list of open questions. Discovery relies on notifications, the chat strip, and needs-input state for parked sessions.
Mode choice is a prompt-level behavior, so tests can prove the guidance exists but not that agents follow it.
New durable tables and outbox reconciliation require SQLite and PostgreSQL migration, race, and restart evidence.
Queue admission can be idempotent. Provider execution still follows the queue's existing ambiguity policy.
Repeated checkpoint reads and a continuation can expose the same correlated answer.

## Alternatives Considered

- Extend the blocking current-turn predicate: this mixes incompatible lifetimes and can reactivate historical waits.
- Return early while retaining the blocking waiter: this leaves pause, expiry, and response recovery with the wrong owner.
- Send a normal peer message when answered: this lacks an atomic outcome/outbox boundary and can prompt or interrupt a running session.
- Keep questions in plan text: this lacks canonical correlation, withdrawal, access control, and Inbox visibility.
- Add another provider-specific push channel: existing MCP reads and queued continuations satisfy the request without that dependency.
- Gate async tools on the Inbox flag: this is smaller, but it keeps the feature off for production users until the Inbox ships.
- Remove the Inbox surface entirely: this loses the cross-task view that issue #4836 asks for, and it discards working code.
- Add a pending-action value for open questions: pending action means a session is blocked, and a running agent is not.

## Records

- [Requirements](../specs/tasks/requirements/async-agent-questions.md)
- [System design](../specs/tasks/system-design/async-agent-questions.md)
- [Plan](../plans/async-agent-questions/plan.md)
