---
status: draft
system: tasks
created: 2026-09-07
owners:
  - kandev
---

# Runner Switch Before Materialization Action Requirements

## Overview

The runner switch action itself: how a runner change is authorized, serialized,
applied, and refused. The mutability rule the action enforces is in
[Runner switch before materialization](runner-switch-before-materialization.md);
what a successful switch changes, and the surface a user drives it from, are in
[Runner switch before materialization effects](runner-switch-before-materialization-effects.md).

This file exists because the parts together exceed the requirement file-size
limit. The split is by layer, not by importance: every acceptance criterion
keeps the identifier it had when they were one document.

## Requirements

### REQ-TASKS-RUNNER-SWITCH-002: Runner switch action

**Intent:** Changing a task's runner is one authorized, atomic, retry-safe
operation that either moves the runner or explains precisely why it cannot,
and that never damages the rest of the task's metadata.

#### Acceptance criteria

- **AC-TASKS-RUNNER-SWITCH-002.1:** The system shall expose a task action named
  `task.runner` accepting a payload with `id` naming the task and
  `executor_profile_id` naming the target runner. The action name shall consist
  of the task namespace plus exactly one further segment, so that the existing
  authorization backstop for top-level task actions applies and promotes `id`
  to the task identifier.
- **AC-TASKS-RUNNER-SWITCH-002.2:** The system shall authorize the caller against
  the named task before evaluating either gate, at the position
  AC-TASKS-RUNNER-SWITCH-002.17 fixes. This authorization shall run for every
  caller on every transport, including the WebSocket gateway. The gateway-level
  backstop is an additional layer for that one transport and shall never be the
  only authorization for the operation: the service shall not vary its
  authorization by the transport a request arrived on, and shall not depend on
  being able to determine that transport.
- **AC-TASKS-RUNNER-SWITCH-002.3:** When the action executes, the system shall
  re-evaluate every condition of AC-TASKS-RUNNER-SWITCH-001.3 inside a single
  transaction that is serialized against session creation and task lifecycle
  cleanup for the same task.
- **AC-TASKS-RUNNER-SWITCH-002.3a:** The serialization rule is general and is
  stated once, because enumerating a subset of the conditions is what lets the
  remainder race. **Every writer that can cause any condition of
  AC-TASKS-RUNNER-SWITCH-001.3 to begin holding for a task, or that can change
  which repository the compatibility gate of AC-TASKS-RUNNER-SWITCH-002.7
  resolves against, shall take the same task-scoped lock the switch takes.** The
  second clause is not a separate rule: a verdict computed against a repository
  the task no longer has is as wrong as a gate condition that began holding
  unobserved, and a writer that swaps a repository link in place changes no row
  count, so the first clause alone would not reach it. No condition is exempt on
  the ground that its row is reached through a session: none of the rows the gate
  reads is keyed to a session, and a condition that is only observed rather than
  serialized is not enforceable. The guarantee
  AC-TASKS-RUNNER-SWITCH-002.13 gives for sessions shall hold identically for
  every writer in scope: the switch commits first and the writer proceeds against
  the new runner, or the writer commits first and the switch is refused with that
  condition's code. A writer added later that can make a gate condition hold, or
  can change the resolved repository, falls under this criterion without it being
  amended.
- **AC-TASKS-RUNNER-SWITCH-002.3b:** The set of writers in scope for
  AC-TASKS-RUNNER-SWITCH-002.3a is finite, is determined by audit rather than by
  assumption, and shall be enumerated so the obligation is bounded and testable
  rather than left for whoever implements it to discover. A writer satisfies the
  rule in one of two ways, and the distinction is what bounds the work:

  1. **Already satisfied, no change required** — the writer updates the task's own
     row, and so takes that row's lock inherently, or takes the task-scoped lock
     explicitly. Archiving the task, writing the task's host workspace path, and
     writing the task's workspace mode are all writes to the task row. Session
     creation and task environment creation take the task-scoped lock explicitly.
  2. **Requires the lock to be added** — the writer writes a different table and
     takes no task-scoped lock today. This capability shall bring each of these
     under the same lock: the running-executor record write (condition 6); the
     workspace source attachment that creates repository links and workspace
     folder attachments (conditions 2, 3 and 7), which today takes the lock only
     when it is also checking an expected parent and therefore takes none at all
     for a task with no parent — the shape this capability most commonly serves;
     the workspace group membership insert (condition 9), which is owned by the
     office system rather than the task system; and the in-place update of an
     existing task-repository link, which is in scope under the second clause of
     AC-TASKS-RUNNER-SWITCH-002.3a.

  Each writer in class 2 shall have its own acceptance evidence: a demonstration
  that it and a concurrent switch produce one of the two outcomes
  AC-TASKS-RUNNER-SWITCH-002.3a permits and never a third. For the three writers
  that make a gate condition begin holding, those two outcomes are the ones that
  criterion names: switch first and the writer proceeds against the new runner, or
  writer first and the switch is refused with that condition's code. The in-place
  link update has no condition code, because it makes no condition hold; its two
  outcomes are that the switch commits first and the link update then applies, or
  the link update commits first and the switch is refused as the retriable
  `evaluation_unavailable` of AC-TASKS-RUNNER-SWITCH-002.7c. Both are outcomes the
  closed vocabulary already contains; this criterion introduces none. A writer in class 1
  shall have the same evidence, but needs no code change to produce it. An
  implementation that changes no writer cannot satisfy this criterion, and one
  that reports the rule as met without per-writer evidence has not met it.
- **AC-TASKS-RUNNER-SWITCH-002.4:** When a switch succeeds, the only task
  metadata that changes shall be the stored executor profile identifier. Every
  other metadata entry, including the agent profile identifier and any deferred
  launch record, shall be unchanged afterwards.
- **AC-TASKS-RUNNER-SWITCH-002.5:** When a switch is rejected for any reason,
  the system shall persist no change to the task.
- **AC-TASKS-RUNNER-SWITCH-002.6:** When a switch is rejected because the
  mutability gate fails, the system shall return a typed conflict carrying the
  same reason code, chosen by the same ordering, that
  AC-TASKS-RUNNER-SWITCH-001.3 defines.
- **AC-TASKS-RUNNER-SWITCH-002.7:** When the mutability gate passes but the
  target runner requires a git clone URL and the task's repository provides
  none, the system shall reject the switch with the typed conflict
  `target_cannot_materialize_repository`. This code shall never appear in the
  projection defined by REQ-TASKS-RUNNER-SWITCH-001, because it describes the
  target rather than the task.
- **AC-TASKS-RUNNER-SWITCH-002.7a:** Clone-URL resolution shall distinguish a
  determinate answer from a failed lookup. A candidate that is absent, empty, or
  whitespace-only supplies no URL and resolution continues to the next candidate.
  Only an exhausted candidate list shall reject with
  `target_cannot_materialize_repository`. A candidate that cannot be evaluated,
  because the lookup errored or timed out, shall reject with
  `evaluation_unavailable` per AC-TASKS-RUNNER-SWITCH-002.18. A transient failure
  to read a repository's origin shall never be reported as a permanent statement
  that the repository cannot be moved.
- **AC-TASKS-RUNNER-SWITCH-002.7b:** The compatibility gate shall not be
  evaluated while the task row lock is held. The target profile's executor, its
  clone-URL requirement, and any repository clone URL shall be resolved before
  the transaction of AC-TASKS-RUNNER-SWITCH-002.3 opens, so no filesystem or
  subprocess call occurs inside the locked section. Resolution shall be bounded
  by a timeout; exceeding it is a failed lookup under
  AC-TASKS-RUNNER-SWITCH-002.7a. What may be resolved early is only what the
  filesystem or a subprocess is needed for; which repository the task has is a
  task-scoped fact and is governed by AC-TASKS-RUNNER-SWITCH-002.7c, not by this
  criterion. Resolving early does not move the decision: the
  resolved result is applied at stage 6 of AC-TASKS-RUNNER-SWITCH-002.17, so a
  task that fails the mutability gate reports that gate's code even when the
  target is also incompatible.
- **AC-TASKS-RUNNER-SWITCH-002.7c:** A compatibility verdict is only valid for
  the repository it was computed against, because it is resolved before the
  transaction that validates which repository the task has. The system shall
  record the identity of the repository the resolution used, and inside the
  transaction, after the mutability gate has passed, shall confirm that the
  task's single attached repository is still that same repository. This
  confirmation shall compare both the repository identity and the link's
  last-updated column, `task_repositories.updated_at`, against the values
  observed at resolution time, so that a link mutated in place is detected even
  though its identity column may not have moved. This confirmation is a
  task-scoped read of two columns and involves no filesystem or subprocess call,
  so it does not violate AC-TASKS-RUNNER-SWITCH-002.7b. It is a second line of
  defence rather than the primary one: the writer that mutates the link is
  itself in scope for AC-TASKS-RUNNER-SWITCH-002.3a, and the confirmation exists
  so that a writer added outside that rule is still caught. When the repository has changed, the resolved
  verdict shall be discarded and the request shall reject as
  `evaluation_unavailable` under AC-TASKS-RUNNER-SWITCH-002.18, which is
  retriable, rather than applying a verdict computed against a repository the
  task no longer has. When the task does not have exactly one repository
  attachment at resolution time, resolution shall be skipped and shall yield no
  verdict; the request then reports `no_repository` or `multiple_repositories`
  from the mutability gate at stage 5, and skipping shall never itself produce an
  outcome. This keeps the ordering of AC-TASKS-RUNNER-SWITCH-002.17 intact: the
  compatibility gate can only ever speak at stage 6.
- **AC-TASKS-RUNNER-SWITCH-002.8:** An identifier in this payload is **blank**
  when it is absent, empty, or contains only whitespace. That single definition
  governs every identifier the action accepts, so `id` and `executor_profile_id`
  are judged by the same test and a whitespace-only value is never treated as a
  value that could be looked up. When either identifier is blank the system shall
  reject the request as invalid. A blank `executor_profile_id` shall never be
  interpreted as clearing the task's runner, and a blank `id` shall be reported
  as invalid rather than as not found.
- **AC-TASKS-RUNNER-SWITCH-002.9:** When the payload names a task that does not
  exist, the system shall reject the request as not found, distinctly from every
  conflict code.
- **AC-TASKS-RUNNER-SWITCH-002.10:** When the payload names an executor profile
  that does not exist, or one whose executor is not available for use, the
  system shall reject the request as invalid, distinctly from every conflict
  code. An executor is available for use when it is not soft-deleted **and** its
  status is active; failing either test makes it unavailable, and both produce
  the same invalid outcome.
- **AC-TASKS-RUNNER-SWITCH-002.11:** When a switch names the executor profile
  the task already stores, the system shall report success, shall leave the
  stored value unchanged, and shall publish a task-updated event only when the
  stored value actually changed. This success is reported only after both gates
  have passed; naming the already-stored profile shall not bypass either. A task
  that has since materialized therefore receives the gate's conflict code, not
  success, even when the requested profile equals the stored one.
- **AC-TASKS-RUNNER-SWITCH-002.12:** When two switches for the same task run
  concurrently, the system shall serialize them on the task. Both may succeed;
  the stored runner afterwards shall be the one written by the transaction that
  committed second, and no intermediate or blended metadata state shall be
  observable.
- **AC-TASKS-RUNNER-SWITCH-002.13:** When a switch and a session creation for
  the same task run concurrently, the system shall produce exactly one of two
  outcomes: the switch commits first and the session is then prepared with the
  new runner, or the session commits first and the switch is rejected with
  `session_exists`. The system shall never leave a changed runner alongside a
  session that was prepared under the previous runner.
- **AC-TASKS-RUNNER-SWITCH-002.14:** When a caller repeats the same switch after
  an ambiguous failure, without supplying any idempotency token, and the task's
  materialization state has not changed in between, the system shall reach the
  same terminal state as a single successful call. The action assigns a value
  rather than accumulating one, so retries need no deduplication key. When the
  task did materialize in between, the repeat is refused by the gate per
  AC-TASKS-RUNNER-SWITCH-002.11; the stored runner remains the one the first call
  committed, so only the reported outcome differs.

- **AC-TASKS-RUNNER-SWITCH-002.15:** When a switch and an archive of the same
  task run concurrently, the system shall serialize them: either the switch
  commits before the archive, or the switch is rejected with `task_archived`. A
  switch shall never commit against an archived task.
- **AC-TASKS-RUNNER-SWITCH-002.16:** The comparison in
  AC-TASKS-RUNNER-SWITCH-002.11 shall be against the value the task stores, not
  against the runner the task would otherwise resolve. Storing an explicit
  executor profile on a task that previously stored none is a change even when
  the named profile is the one the workspace default would have selected.
- **AC-TASKS-RUNNER-SWITCH-002.17:** When more than one rejection class applies
  to the same request, the system shall report the outcome of the **first**
  failing stage in exactly this order, so that a given request always produces
  the same outcome regardless of implementation:

  1. **Malformed request** (invalid): `id` or `executor_profile_id` blank in the
     sense AC-TASKS-RUNNER-SWITCH-002.8 defines. Evaluated before any lookup, so
     a blank identifier never reaches the not-found or target-invalid stages.
  2. **Task not found** per AC-TASKS-RUNNER-SWITCH-002.9.
  3. **Caller not authorized** per AC-TASKS-RUNNER-SWITCH-002.2, after existence
     so the outcome matches the other top-level task actions.
  4. **Target invalid** per AC-TASKS-RUNNER-SWITCH-002.10.
  5. **Mutability gate** per AC-TASKS-RUNNER-SWITCH-002.6, itself ordered by
     AC-TASKS-RUNNER-SWITCH-001.3.
  6. **Compatibility gate** per AC-TASKS-RUNNER-SWITCH-002.7.
  7. **Assignment**, including the no-op of AC-TASKS-RUNNER-SWITCH-002.11.

  An evaluation failure under AC-TASKS-RUNNER-SWITCH-002.18 may arise at stages
  2 and 4 through 7 and preempts the outcome of the stage where it occurs. Stage
  7 is included because that criterion covers the write and the commit, not only
  the reads: a request that passed every gate and then failed to commit reports
  the retriable outcome rather than a success or a conflict.
- **AC-TASKS-RUNNER-SWITCH-002.18:** When the system cannot complete an
  operation it needs in order to decide or to apply the request, the action shall
  reject with a typed `evaluation_unavailable` outcome, distinct from every
  mutability conflict code, from `target_cannot_materialize_repository`, and from
  the invalid and not-found outcomes. It shall be marked retriable and shall
  persist no change per AC-TASKS-RUNNER-SWITCH-002.5. **This covers writes and
  not only reads:** a failure to acquire the task-scoped lock, a failure of the
  metadata write, and a failure to commit the transaction all produce this
  outcome, exactly as a failed read does. Naming only reads would leave the write
  path with no outcome class at all, and the caller would receive something
  outside the vocabulary AC-TASKS-RUNNER-SWITCH-004.4b is required to present.
  Such a failure shall never be reported as a mutability conflict: a conflict
  says the task is ineligible, a failed operation only that the system could not
  complete it. A caller that receives it may repeat the request under
  AC-TASKS-RUNNER-SWITCH-002.14, which is why this outcome and only this outcome
  is retriable.
- **AC-TASKS-RUNNER-SWITCH-002.19:** A task-updated event for a runner change
  shall carry the task's updated-at value from the committing transaction, and a
  client shall ignore an event whose updated-at is older than the value it holds
  for that task. Clients receiving two switches' events out of order shall
  converge on the value written by the transaction that committed second, per
  AC-TASKS-RUNNER-SWITCH-002.12. Ordered delivery is not required; a late event
  must not overwrite a newer one.
