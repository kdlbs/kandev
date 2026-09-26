---
status: current
system: tasks
requirements:
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002
---

# Administrative turn settlement System Design

## Purpose and boundaries

The task repository owns the durable completion-intent and session-control
records; the reconciler owns the exact compare-and-set transition; the task
MCP server owns the narrow settlement tool and trusted caller identity. The
explicit completion signal contract remains
[Workflow explicit completion signal](requirements/workflow-explicit-completion-signal.md);
the delivery receipts that report across tasks remain
[Recoverable cross-task delivery](requirements/recoverable-cross-task-delivery.md).

The prior system record for the capability is the legacy
[Administrative Turn Settlement spec](../../workflow/administrative-turn-settlement/spec.md)
and [ADR-2026-08-19 durable administrative turn settlement](../../../decisions/2026-08-19-durable-administrative-turn-settlement.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001 | Components; Authority and evidence |
| REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002 | Reconciler control flow; Supersession |

## Components

- `internal/orchestrator/stale_session_settlement.go`: request validation,
  evidence evaluation, and the settlement commit path.
- `internal/orchestrator/completion_settlement.go`: intent reconciliation
  including the superseded outcome and the atomic control-event transition.
- `internal/mcp/handlers/settle_stale_session.go`: trusted caller identity
  resolution, the narrow relation authority matrix, and refusal audit.
- `internal/mcp/server/server.go` (task mode only): tool registration with
  server-derived `sender_task_id`/`sender_session_id`; the request body can
  name only the target `session_id` and `turn_id`.
- `internal/agent/runtime/lifecycle/session_launch.go`: write-once
  `persistSpawnSupervision` provenance that back the `persisted_supervisor`
  authority basis.

## Authority and evidence

`staleSettlementAuthority` grants exactly three bases:
same-workspace peer session on the target task (`same_task_peer`), the
target's direct parent (`direct_parent`), or a server-recorded spawn
supervisor matched on both task and session identity
(`persisted_supervisor`). The supervisor record is written once at spawn
time from server-verified identity, never from tool arguments, and a
conflicting existing record fails closed.

`staleSettlementCandidate` then demands durable terminal evidence before any
mutation: a running session, the exact authoritative active turn, no prompt
reservation, no background work, no persisted background attestation, no
pending tool calls, and an eligible pending completion intent whose quiet
grace has passed. A CREATED-state session with no turn rows is refused as
`session_not_running`/`not_stale` with zero mutation; the refusal record
carries no launch-authority claim. Turn authority itself excludes pure
dispatch-pending reservations, so process materialization alone can never
look like turn evidence.

## Reconciler control flow

Settlement takes the same in-flight cancellation guard as the ordinary stop
path, evaluates the candidate under the guard, then delegates the terminal
commit to `reconcileCompletionIntentLocked`. For an authorized manual
settlement, one repository transaction completes the captured turn, transitions
its completion intent, and inserts the audit event. The transaction reads the
intent's captured step and locks the task row before choosing `settled` or
`superseded`; a task move committed before that choice supersedes the old
intent. A failed write leaves all three durable records unchanged. After
commit, the normal turn-completed event is published and the reconciler
releases session ownership. Workflow evaluation also checks the captured step
before running, so a move immediately after commit cannot complete the
destination step. A post-commit readback that finds the intent not in a
terminal state refuses with `settlement_not_committed`.

## Supersession

When the task moved before reconciliation, the intent settles as
`superseded`: the old workflow step is never re-evaluated and only the
current transition's on-entry delivery is delivered once. Late frames,
user activity, or a successor turn rearm or reject the intent so the older
identity can never close a successor.

## Data and contracts

`session_completion_intents` holds one row per (session, turn, workflow
step) with captured execution identity, summary, state (`pending`,
`settling`, `settled`, `reopened`, `superseded`, `rejected`), and activity
timestamps. `session_control_events` records authorized settlement attempts
and refusals with actor, target, authority basis, evidence code, and result,
without prompt content. `session_spawn_supervision` in session metadata
stores the immutable write-once supervisor provenance.

## Failure and recovery

Refusals never mutate durable state and are auditable as `not_stale`
denials; a retry after a completed settlement returns `already_settled`.
A failure during manual settlement cannot leave only the turn, intent, or audit
committed because their terminal writes share one transaction. Startup
reconciliation uses the same durable predicate as the manual tool, so a
duplicate attempt cannot close a successor or re-run a transition.

## Security

Caller identity is injected by the task-mode MCP server, never read from
the request body. The settlement tool is task-mode only. Denials are
recorded with the densiest granularity available (evidence code and
authority basis) without prompt content, and the `unattributed lifecycle
anomaly` classification stays distinct from any launch-authority verdict.

## Observability

`administrative_turn_stale_control_denials_total` counts denials by reason
(`cancellation_in_flight`, `already_settled`, `session_not_running`,
`successor_or_missing_turn`, `prompt_reservation`, `background_work`,
`background_work_attested`, `pending_tool_call`, `completion_evidence_missing`,
`relation_denied`, `settlement_not_committed`). The refusal and audit tables
carry the durable evidence trail.

## Related decisions

- [ADR 2026-08-19 durable administrative turn settlement](../../../decisions/2026-08-19-durable-administrative-turn-settlement.md)
