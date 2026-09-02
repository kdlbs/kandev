---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
created: 2026-09-01
owners:
  - Kandev team
---

# Prompt Attachments System Design

## Purpose and boundaries

The task system owns prompt-attachment staging, claims, delivery descriptors,
and retention. It also owns admission of those descriptors to a task session.

The agent runtime materializes claimed files and sends them with the prompt.
It cannot claim staged files or infer task ownership.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-PROMPT-ATTACHMENTS-001` | Claim admission, materialization and delivery, failure and recovery, security |

## Components and responsibilities

- The attachment service stores staged files and validates the authenticated
  owner, workspace, task, and optional session.
- The task repository changes a complete attachment set from `staged` to
  `claimed` in one transaction.
- The orchestrator admits `LaunchSessionRequest.Attachments` before any agent
  start or prompt-turn creation.
- The lifecycle manager reads claimed files and streams them to agentctl before
  it sends the prompt.
- The orchestrator converts a rejected initial prompt into the durable launch
  failure state.

## Claim admission

`LaunchSessionRequest.Attachments` contains untrusted attachment identifiers.
The orchestrator authorizes the task and optional session before each claim.

The orchestrator uses a narrow attachment-claimer interface. The task service
implements this interface and derives the owner and workspace from server
state.

The launch entry point claims attachments before it calls an intent handler.
This order prevents invalid descriptors from starting an agent or prompt turn.

A launch with an existing session uses that session as the claim scope. A new
session launch can use a task-scoped claim because no session identity exists.

Task creation also creates task-scoped claims before it prepares a session. A
later launch for that task treats the existing claim as idempotent.

A session-scoped claim remains bound to its session. Another session cannot use
that claim, even when both sessions belong to the same task.

## Materialization and delivery

The lifecycle manager accepts only claimed file descriptors. The attachment
reader checks the task and the optional session before it opens a file.

The lifecycle manager streams each file to the active agentctl instance. It
then uses the returned safe name for native-prompt or workspace-path delivery.

The lifecycle manager starts prompt generation before materialization. A
materialization error therefore uses the same terminal prompt-error path as an
ACP submission error.

## Failure and recovery

A claim error returns from `session.launch` before the intent changes runtime
state. The response does not disclose another owner, workspace, task, or path.

A materialization or ACP submission error can occur after the launch response.
The lifecycle manager reports that error with the current execution identity.

The existing agent-failure path settles the current turn and session with a
safe generic error. It also publishes the durable task and session state used
by the chat surface.

A delayed error cannot settle a replacement execution or successor prompt. The
existing execution and prompt evidence checks reject stale terminal events.

Shutdown cancellation remains a stopped execution. It does not become a user
visible launch error.

## Persistence

The attachment registry is the source of truth for claim state. Claim changes
are transactional and survive backend restarts.

Claim admission does not write file bytes to a task message. The initial user
message stores bounded attachment descriptors after the launch succeeds.

## Security

Clients cannot provide an owner, workspace, storage key, or executor path. The
backend derives those values from authenticated and persisted state.

Task-scoped idempotency applies only when the stored task matches. Session
scoping remains strict when the stored claim contains a session identity.

## Observability

Existing launch request diagnostics correlate claim errors with their task and
session. They do not include attachment bytes or storage paths.

Initial prompt errors retain the execution identity. The terminal event and
durable state provide evidence that the prompt did not remain active.

## Related decisions

- [ADR-2026-08-04-file-backed-prompt-attachments](../../../decisions/2026-08-04-file-backed-prompt-attachments.md)
- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)
