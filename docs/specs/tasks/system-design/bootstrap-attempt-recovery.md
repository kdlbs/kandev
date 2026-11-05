---
status: draft
system: tasks
created: 2026-10-22
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005
---

# Bootstrap attempt recovery system design

## Boundary and mapping

This extends [task launch recovery](task-launch-failure-recovery.md) at the failed-attempt lifecycle boundary.
The existing design is near its size limit, so this extension has a separate design file.
REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005 maps to attempt evidence, recovery admission, queue ownership, and projection below.
It applies to unwrapped agents, generic wrappers, and sandbox providers.

## Observed source

`orchestrator/task_operations.go::resumeRequiresExecutorRecord` rejects ordinary waiting sessions with no running record.
`attemptColdResume` classifies missing records as awaiting launch.
The retained Task 04 instance exhibits both paths after a bootstrap authentication error.
Its build predates the current parent. Reproduce this shape on current source before changing behavior.

Existing `queue_user_prompt_runtime_unavailable_test.go` protects prompts while real startup remains pending.
Preserve those tests. A missing row is insufficient evidence of either failure or an active launch.

## Attempt evidence

Persist an explicit bootstrap outcome with the existing execution/generation identity before publishing the failure.
Store a safe category, failure phase, timestamp, recovery stamp, and authoritative snapshot references.
Preserve raw provider diagnostics only in their existing private bounded diagnostic channel.
Authentication-required errors must remain actionable without echoing provider messages or credential paths.

A valid launch-in-progress classification requires an owned startup attempt or durable creating state with a live owner.
A failed bootstrap is a separate condition even when its session remains waiting for input.
Legacy rows without sufficient evidence remain recovery-blocked. Do not backfill success or guess ownership.

## Recovery admission

Explicit recovery verifies caller access, task/session identity, error stamp, execution generation, and absence of a newer live attempt.
Use the existing per-session resume admission and execution fences.
Allow missing-executor reconstruction only for the classified failed attempt with complete authoritative snapshots.
Revalidate repository, workspace, agent, executor, and sandbox revision eligibility.
Do not convert arbitrary database lookup errors into missing-row recovery.
Do not weaken policy or create a new conversation to make a retry appear successful.

After configuration, wait for runtime readiness before clearing the current active error.
Retain historical error metadata under the existing error-history contract.
If startup fails again, retain the updated typed cause on the same session.
A stale completion from the previous attempt cannot update the successor.

## Queue ownership

Keep queued messages bound to session incarnation and existing admission/dispatch receipts.
A bootstrap-blocked queue has no automatic retry wake until explicit successful recovery.
Show that state through the existing queue projection and recovery surface.
The operator can inspect, edit, or remove queued work through existing authorized actions.
Do not submit the queue to a successful sibling or automatically copy prompts into a replacement session.
Recovery success resumes normal admission. Existing claim tokens enforce at-most-once dispatch.

## Projection and UI

Reuse `components/task/chat/session-recovery-card.tsx`, task status summaries, and existing error-history/dismissal behavior.
Show the current session's cause and blocked queued count.
A sibling success can change task workflow state without changing the original session's failure ownership.
Do not clear an error merely because the task reaches Review.
Desktop and phone use the same eligibility and state hooks.
Phone actions retain visible touch controls, safe-area handling, and at least 44px targets.
See UI-04 in the [plan](../../../plans/nono-sandbox-integration/plan.md).

## Validation

Reproduce authentication failure after ACP initialization, remove the running record through normal cleanup, then recover the same session.
Assert queue retention, no retry loop, one dispatch after readiness, and stale-attempt rejection.
Pair with a genuinely starting session that still queues normally.
Cover database read failures, missing snapshots, concurrent recovery, cancellation, sibling success, and policy-required snapshots.
Run the exact targeted commands in Task 02. No runtime changes belong to this design turn.
