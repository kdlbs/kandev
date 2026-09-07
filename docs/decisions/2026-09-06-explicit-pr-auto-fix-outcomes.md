# ADR-2026-09-06-explicit-pr-auto-fix-outcomes: Require Explicit Outcomes for PR Auto-Fix Attempts

**Status:** proposed
**Date:** 2026-09-06
**Area:** backend, frontend, protocol, workflow, GitHub

## Context

GitHub PR auto-fix records its feedback signature and round count as soon as a
prompt is accepted or durably queued. Later watcher evaluations suppress the
same signature.

That rule prevents duplicate prompts while the agent is working, but it also
treats prompt delivery as successful remediation. If the agent ends its turn
without changing provider state, the same failed checks remain permanently
suppressed. Agent prose is not a reliable completion signal because it can
claim commands or provider actions that did not occur.

Retrying every unchanged snapshot is also unsafe. Some feedback is genuinely
non-actionable or blocked outside the task, and blind retries would consume all
10 rounds without adding information.

## Decision

Every GitHub PR auto-fix prompt creates one durable attempt. The attempt moves
through `queued`, `running`, `awaiting_provider_progress`, `acknowledged`, or
`retryable`. Feedback deduplication coalesces only while the attempt is queued,
running, awaiting provider progress inside its deadline, or acknowledged.

Kandev binds a delivered attempt to its exact task, session, turn, linked PR,
and feedback signature. A new task-bound MCP tool,
`report_pr_auto_fix_outcome_kandev`, accepts one first-write outcome from that
turn:

- `action_taken`: wait for provider-visible progress;
- `non_actionable`: acknowledge the unchanged snapshot; or
- `blocked`: acknowledge the snapshot and expose the bounded reason through
  the existing per-PR automation error surface.

The server appends the outcome protocol to every auto-fix prompt as immutable
context. Saved global prompts and task overrides remain customizable but cannot
remove, replace, or retarget the protocol. Structured sessions receive trusted
hidden context. Passthrough sessions receive equivalent plain terminal input
because their transport cannot hide system blocks.

A successful turn or recoverable agent failure without a valid outcome makes
the matching attempt retryable. `action_taken` waits for provider progress for
two complete one-minute watch intervals. A changed PR head, check execution
timestamp, review-thread state, conflict state, or queue-removal state confirms
progress. If none changes before the deadline, the attempt becomes retryable.
Check execution timestamps are part of provider generation so rerunning the
same named check can rearm auto-fix even when its final text is unchanged.

The next settled watcher evaluation creates a retry round for a retryable
snapshot. Retries use the existing per-PR 10-round budget and never bypass the
cap. User cancellation, disabling auto-fix, task archive/deletion, and terminal
PR state do not start a retry.

Startup reconciliation preserves queued attempts and marks a running attempt
retryable when its bound turn is already terminal without a durable outcome.

Untouched legacy `ci-auto-fix` prompt rows are upgraded by exact known content
hash and the `created_at == updated_at` guard. User-edited rows remain intact.

## Consequences

- Prompt delivery no longer counts as remediation completion.
- An agent that silently stops gets another bounded chance without waiting for
  a new feedback signature.
- Explicit non-actionable and blocked outcomes prevent wasteful retry loops.
- The auto-fix store gains durable attempt identity and state. Direct and
  queued delivery must bind the same turn metadata before accepting an outcome.
- The MCP catalog gains one GitHub-provider task tool. It has no caller-supplied
  task, session, turn, or PR selector.
- Normal turn completion and recoverable failure paths gain a narrow auto-fix
  reconciliation call. Workflow transitions and user-originated turns keep
  their existing behavior.
- Existing round badges remain the user-facing retry budget. Help and public
  docs must explain that an undispositioned retry consumes another round.
- GitLab MR auto-fix remains unchanged in this decision. Its shared dispatcher
  must retain provider-neutral behavior so GitLab can adopt the protocol in a
  separate parity change.

## Alternatives Considered

1. **Retry every unchanged snapshot after every turn.** Rejected because
   non-actionable and externally blocked feedback would burn the full budget.
2. **Infer success from final agent prose.** Rejected because prose can be
   incomplete or claim actions that did not occur.
3. **Infer success from shell and MCP tool traces.** Rejected because supported
   agents expose different tools, commands can fail after invocation, and
   provider actions can happen outside the captured trace.
4. **Keep acknowledging at prompt enqueue.** Rejected because it is the source
   of the permanent suppression failure.
5. **Reuse `step_complete_kandev`.** Rejected because workflow-step completion
   and one PR feedback attempt have different identity, lifecycle, and effects.

