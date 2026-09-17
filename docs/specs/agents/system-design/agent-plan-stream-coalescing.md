---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001
---

# Agent Plan Stream Coalescing System Design

## Purpose and boundaries

The Agents system owns conversion of provider updates into durable conversation
messages. This design covers ACP plan correlation, transcript persistence, and
the compatibility projection used by the web conversation. It does not change
task-plan document storage or the plan-card component.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Persistence and compatibility](#persistence-and-compatibility) |

## Components and responsibilities

- `internal/agentctl/server/adapter/transport/acp` extracts `rawInput.plan` from
  ACP tool updates and attaches the source `ToolCallID` to
  `streams.EventTypeAgentPlan`.
- `internal/orchestrator` translates the correlated event into an idempotent
  conversation-message update scoped to the active session and turn.
- `internal/task/service` owns message creation and updates. Its agent-plan
  upsert uses the same durable update path for first delivery, retries, and
  later snapshots.
- `apps/web/hooks/use-processed-messages.ts` projects correlated rows and
  conservatively folds legacy cumulative snapshots before render grouping.
- `AgentPlanMessage` remains the presentation component for the resulting
  message and does not perform deduplication.

## Data and contracts

`streams.AgentEvent` already carries `ToolCallID`. An
`EventTypeAgentPlan` emitted from an ACP tool update shall set:

- `SessionID` to the agent session;
- `ToolCallID` to the source ACP tool call;
- `PlanContent` to the complete plan snapshot from that update.

The orchestrator derives a namespaced plan-message correlation key from
`ToolCallID`. The namespace prevents collision with the visible tool-call
message while preserving the original source identity in message metadata. The
durable message identity additionally includes the session and active turn, so
providers that reuse tool-call identifiers cannot merge unrelated plans.

An uncorrelated `EventTypeAgentPlan` remains supported for adapters that do not
yet provide a tool-call identity. It follows the existing append behavior and
is eligible only for the conservative web compatibility projection.

## Control flow

1. The ACP adapter receives a tool-call update containing a non-empty
   `rawInput.plan`.
2. It emits an agent-plan event with the tool-call identity on every accepted
   snapshot.
3. The orchestrator resolves the active turn and asks the task service to
   upsert the correlated `agent_plan` message.
4. The first snapshot creates the message; later snapshots update its content.
   Delivery order is authoritative because ACP notifications and orchestrator
   handling preserve their FIFO order.
5. WebSocket delivery and conversation replay both expose that same message.
6. Before grouping render items, the web projection retains the latest message
   for a repeated durable correlation key and applies the legacy prefix rule
   only to uncorrelated rows.

## Failure and recovery

Missing session IDs, empty plan content, and unavailable message persistence
remain non-actionable events under the existing handler guard. A correlated
retry is idempotent and cannot create another plan row. If a provider omits
`ToolCallID`, Kandev preserves the plan as a separate message rather than
discarding it.

Legacy folding requires all messages to be `agent_plan` entries in the same
turn, adjacent after normal visibility filtering, and each previous content to
be a strict prefix of the next. Any failed condition ends the chain. This
failure-closed rule may leave ambiguous duplicates visible but cannot merge
unrelated plans.

## Persistence and compatibility

No schema migration is required. The existing message content and metadata
fields store the latest snapshot and its correlation identity. Upsert identity
is deterministic across retries and process restarts.

Existing duplicate rows remain unchanged. The web compatibility projection
selects the final row from qualifying legacy chains, which preserves immutable
history while repairing conversation readability on live and paginated reads.

## Observability

Existing agent event and message update logs expose the event type, task,
session, and tool-call update outcome. Failures use the current
`failed to create agent plan message` error path, renamed to describe the
upsert operation when correlation is present.

## Responsive presentation

This change only normalizes transcript data before the existing plan card is
rendered. Desktop and phone use the same message hook and therefore receive the
same one-card projection. The plan-card composition, scroll owner, controls,
touch targets, safe-area behavior, and navigation do not change. Targeted hook
tests satisfy mobile parity because no viewport-dependent UI behavior changes.

## Related decisions

- [ADR-2026-09-17-coalesce-streamed-agent-plans](../../../decisions/2026-09-17-coalesce-streamed-agent-plans.md)
