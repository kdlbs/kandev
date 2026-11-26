---
status: draft
system: tasks
requirements:
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
---

# Interrupted task warning design

## Ownership and data

The task system owns the warning. Startup and periodic reconciliation record
lost mid-turn work. `reconcileActiveSessionOnStartup` and `markTaskInterrupted`
retain their existing liveness, state, archive, and recovery-settlement guards.

Keep the scalar `tasks.metadata.interrupted_at` and the task DTO field
`interrupted`. Add a private `interrupted_sessions` metadata map on the task.
Each key identifies an interrupted session. Each value records its incarnation,
interruption time, and an immutable interruption token. A new interruption
replaces only the entry for its session. Repeating the same settlement keeps the
same entry. The scalar records the latest time among the remaining entries.

The repository changes the map and scalar together under the task row write
boundary. The last removal deletes both keys. This keeps sidebar, board, and
boot reads on the existing projection without joins or a schema migration.
Timestamp-only records continue to project a warning. When such a record is
the only evidence, the first new accepted prompt on that task can clear it by
comparing the captured scalar. A scoped interruption takes precedence over
this legacy fallback.

## Warning resolution

`markRecoveryResolvedForAttempt` records a successful agent boot and settles
the existing error and recovery state. The boot callback leaves the task warning
in place. This applies to startup recovery, focus recovery, and explicit
prompt-free Resume. A restored conversation can wait for its next prompt and
still show the warning.

Capture the marker for the affected session just before prompt dispatch. Bind
it to the task, session incarnation, current attempt, and execution. For an
ordinary or queued prompt, use the existing admission and accepted-dispatch
callbacks in `task_operations.go`. For a prompt carried by a replacement
launch, use the matching initial prompt admission and acceptance callbacks.
Both paths reuse the same conditional removal. Follow [the accepted-prompt
decision](../../../decisions/2026-10-20-silent-prompt-acceptance.md) for the
acceptance boundary. The warning does not depend on visible model output.

Only a positively acknowledged prompt for the captured owner can remove its
entry. Queue insertion, generic session state, lifecycle-only turns, and
replayed events provide no acceptance evidence. When the agent accepts an
eligible queued prompt, normal delivery can clear the warning. The interrupted
prompt retains its stored outcome.

The repository compares the captured interruption token and session
incarnation in the write. A delayed callback can neither remove a newer entry
nor remove another session's entry. Session deletion removes the matching
incarnation's entry in its existing transaction. Existing archive and terminal
rules determine whether any warning can be visible.

Publish the committed task through the ordered `task.updated` path when the
projected warning changes. Clients retain their marker on a partial update that
omits it. If acceptance is uncertain or the marker write fails, retain the
warning. Treat a marker-write error as a projection error. It must not turn an
accepted prompt into a failed delivery or send the prompt again.

## Shared rendering

Reuse `InterruptedTaskIcon`, the warning color, the test ID
`task-state-interrupted`, and the localized `common:interruptedByRestart` label.
The sidebar and card show the triangle in the existing status slot when the
task is waiting for continuation. Permission, clarification, live activity,
and explicit terminal icons retain their existing precedence.

The desktop task sidebar and phone task switcher use the same task projection.
The phone switcher opens from the task header and scrolls within its existing
drawer. The row remains the navigation target, and the warning has a distinct
shape and an accessible name. The same state is available on board cards.

## Verification and mapping

| Criteria suffix | Design sections |
| --- | --- |
| 001.2, 001.5, 001.10 | Ownership and data, Warning resolution |
| 001.4, 001.8, 001.9 | Warning resolution |
| 001.3, 001.6, 001.7 | Shared rendering |

The [work order](../../../plans/interrupted-task-warning-continuation/task-01-keep-warning-until-continuation.md)
defines the SQLite and PostgreSQL evidence and the desktop and phone checks.
