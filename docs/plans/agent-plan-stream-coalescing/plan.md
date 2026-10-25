---
status: done
requirements:
  - REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001
system_design:
  - ../../specs/agents/system-design/agent-plan-stream-coalescing.md
---

# Agent plan stream coalescing

## Overview

Turn repeated ACP plan-input snapshots from one tool call into one durable,
updating conversation message. Retain a conservative frontend projection for
historical duplicate rows that lack correlation metadata.

## Delivery order

1. Add a failing ACP adapter test proving an emitted plan event carries the
   source tool-call identity.
2. Add failing orchestrator and task-service tests proving repeated snapshots
   upsert one `agent_plan` message while distinct tool calls remain separate.
3. Implement adapter correlation and durable message upsert.
4. Add failing frontend tests for correlated rows and contiguous legacy prefix
   chains, then implement the compatibility projection.
5. Prove replay behavior through a focused Playwright transcript scenario and
   run the targeted backend, frontend, and specification checks.

The work is one vertical slice because adapter correlation, persistence, and
projection are all required for one stable live-and-replay outcome.

## Technical approach

Use the existing `AgentEvent.ToolCallID` field for
`EventTypeAgentPlan`. The orchestrator shall pass the task, session, active
turn, source tool call, and snapshot to a dedicated task-service upsert path.
The task service derives the deterministic message ID and namespaced
correlation metadata from the session, turn, and source tool call. The
repository serializes the read/create/update decision for that identity in one
transaction; the first event creates one `agent_plan` row and each later
accepted event updates that row's content.

Keep uncorrelated adapter events append-compatible. In
`use-processed-messages`, normalize visible plans before activity grouping:
retain only the newest row for a repeated durable correlation key, and collapse
uncorrelated rows only when they are adjacent, share a turn, and form a strict
content-prefix chain.

Do not add a database migration or modify `AgentPlanMessage`.

## ASCII UI preview

`UI-01: Streamed agent plan transcript`, entry point: desktop or phone task
conversation after one plan-producing tool call.

Current observed behavior:

```text
Conversation (single scroll owner)
+ Agent plan: Batch ES reads       [collapsed]
+ Agent plan: Batch ES reads       [collapsed]
+ Agent plan: Batch ES reads       [collapsed]
+ ...one card per partial snapshot
```

Proposed shared desktop and phone behavior:

```text
Conversation (single scroll owner)
+ Agent plan: Batch ES reads       [collapsed]
  latest complete snapshot
```

The one-card cardinality is required by
`AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1` and `.5`. Spacing, card
geometry, collapse controls, scroll behavior, and touch targets are unchanged.

## Verification strategy

- ACP adapter unit tests cover `ToolCallID` propagation and empty-plan guards.
- Task-service and orchestrator tests cover create, update, identical no-op,
  retry, distinct tool calls, serialized concurrent updates, and uncorrelated
  compatibility. A PostgreSQL multi-connection regression proves the identity
  lock spans the read/update transaction.
- Frontend unit tests cover correlated duplicates, legacy prefix chains, turn
  and adjacency boundaries, and non-prefix plans.
- One Chromium Playwright scenario loads persisted duplicate snapshots and
  observes one latest plan card. The shared normalization hook makes a second
  mobile scenario redundant because no responsive presentation changes.
- Specification validation and diff checks close the package.

## Work orders

- [x] [Task 01: Coalesce streamed agent plans](task-01-coalesce-streamed-agent-plans.md)

## Verification results

- ACP adapter, task-service, and orchestrator plan regressions passed.
- Agent-plan repository regressions passed, including a real PostgreSQL
  multi-connection lock-order case and the identical-snapshot no-op assertion.
- Task-service and orchestrator plan regressions passed under the Go race
  detector.
- Backend application and integration message adapters compiled.
- 87 focused frontend tests, TypeScript typecheck, and changed-file ESLint
  passed.
- The fresh production-build Chromium replay scenario passed before and after
  reload.

## Risks and exclusions

- A correlation key that is not namespaced can collide with the visible tool
  call message.
- Content-length heuristics can merge unrelated plans; the legacy path must use
  strict adjacency, same-turn, and prefix checks.
- Retried or concurrent delivery can race with message creation and update;
  the persistence boundary must serialize the full mutation by plan identity.
- Do not rewrite historical rows, change task-plan documents, change plan-card
  UI, or broaden transcript deduplication to other message types.
