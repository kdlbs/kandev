---
status: draft
system: tasks
created: 2026-09-07
owners:
  - kandev
---

# Runner Switch Before Materialization Effects Requirements

## Overview

What a runner switch changes, and how a user drives it. The decision contract
itself is split across two neighbours: the mutability projection is in
[Runner switch before materialization](runner-switch-before-materialization.md)
and the switch action is in
[Runner switch before materialization action](runner-switch-before-materialization-action.md).

This file exists because the parts together exceed the requirement file-size
limit. The split is by layer, not by importance: every acceptance criterion
keeps the identifier it had when they were one document.

## Requirements

### REQ-TASKS-RUNNER-SWITCH-003: Effect of a switch on the next launch

**Intent:** A successful switch is observable in exactly one place, the next
session the task prepares, and nowhere else.

#### Acceptance criteria

- **AC-TASKS-RUNNER-SWITCH-003.1:** When a task's runner has been switched and
  the task then prepares its first session, the system shall bind that session
  to the executor profile named by the switch and to the executor that profile
  belongs to. Some launch paths name an executor profile explicitly in the launch
  request rather than leaving it to be read from the task; the task edit dialog is
  one of them. Where a launch is issued by the same interaction that performed the
  switch, the profile it names shall be the switched-to profile, so that an
  explicitly named profile can never disagree with the value the switch just
  committed. A launch issued by some other caller that names a profile explicitly
  is unchanged by this capability and continues to use the profile it names; this
  criterion constrains only the launch that shares an interaction with a switch,
  because that is the only one whose disagreement the switch itself would cause.
- **AC-TASKS-RUNNER-SWITCH-003.2:** When a switch succeeds, the system shall not
  create, modify, or remove any session, task environment, worktree, container,
  or running-executor record.
- **AC-TASKS-RUNNER-SWITCH-003.3:** When a switch changes the stored runner, the
  system shall publish a task-updated event whose payload carries both the new
  executor profile and the recomputed values required by
  AC-TASKS-RUNNER-SWITCH-001.1.

- **AC-TASKS-RUNNER-SWITCH-003.3a:** Publication of that event is best-effort and
  is not part of the committed transaction. A publication that fails shall not
  roll back, retry, or otherwise alter the committed switch, and shall not be
  reported to the caller as a failed switch; the switch succeeded and the stored
  runner is the new one. Because a repeat of the request is a no-op under
  AC-TASKS-RUNNER-SWITCH-002.11 and therefore publishes nothing, the event shall
  not be the only way a client can learn the new value: any subsequent read of
  the task shall carry the committed runner and the recomputed verdict, so a
  client that missed the event converges on its next read of that task rather
  than staying wrong until something unrelated refreshes it. No durable outbox,
  replay log, or delivery receipt is required by this contract.
- **AC-TASKS-RUNNER-SWITCH-003.4:** When the task's workflow step pins an agent
  profile, that pin shall continue to govern the agent profile only. It shall not
  override the executor profile a switch stored.
- **AC-TASKS-RUNNER-SWITCH-003.5:** When a task with a parent has a stored
  executor profile because of a switch, the launch shall use that profile and
  shall not fall back to inheriting the parent's runner.

### REQ-TASKS-RUNNER-SWITCH-004: Runner editing surface

**Intent:** The runner picker that already exists in the task dialog becomes
effective when editing a task, and it is gated by the server's verdict rather
than by a client-side approximation of it.

#### Acceptance criteria

- **AC-TASKS-RUNNER-SWITCH-004.1:** When a user edits a task whose projected
  `runner_editable` is `true`, the task dialog shall offer the executor profile
  selector, and saving a changed selection shall perform the runner switch.
- **AC-TASKS-RUNNER-SWITCH-004.2:** When a user edits a task whose projected
  `runner_editable` is `false`, the dialog shall not offer the selector for
  editing and shall present the projected reason to the user.
- **AC-TASKS-RUNNER-SWITCH-004.3:** The dialog shall decide whether the runner
  is editable only from the projected verdict. It shall not decide editability
  from the task's workflow state, and the previous state-only gate shall no
  longer govern the runner selector.
- **AC-TASKS-RUNNER-SWITCH-004.4:** When a switch is rejected, the dialog shall
  remain open, shall present the returned reason, and shall not show the
  rejected selection as if it had been applied.
- **AC-TASKS-RUNNER-SWITCH-004.4a:** When one save changes the executor profile
  as well as other task fields, the dialog shall issue the runner switch **first**
  and the remaining updates only after it succeeds. When the switch is rejected
  the dialog shall issue no other update, shall leave every other edited field
  unsaved and still populated, and shall report that nothing was saved. A
  rejected switch shall never leave part of the save applied.
- **AC-TASKS-RUNNER-SWITCH-004.4c:** The save is an ordered sequence of
  independent calls and is not atomic, so the rule covers the whole sequence
  rather than only its first step. When any call fails, the dialog shall stop at
  that call and issue none of the calls after it; it shall leave the fields those
  later calls would have saved unsaved and still populated; and it shall report
  which part of the save was applied and which was not, rather than reporting a
  single undifferentiated success or failure. A call that already committed shall
  not be reversed, because no compensating action exists for it: a committed
  runner switch stays committed even when a later field update fails, and the
  dialog shall present that state truthfully instead of implying the whole save
  was rejected. Retrying such a save is safe in the sense that it cannot
  double-apply the switch, because the switch is a value assignment rather than an
  accumulation (AC-TASKS-RUNNER-SWITCH-002.11, AC-TASKS-RUNNER-SWITCH-002.14). It
  is not guaranteed to succeed: if a later call in the failed sequence materialized
  the task, the repeated switch is refused by the mutability gate with that
  condition's code even though the stored runner is already the intended one. The
  dialog shall present that refusal as the gate outcome it is, and shall not
  present it as the switch having failed to apply.
- **AC-TASKS-RUNNER-SWITCH-004.4b:** When the outcome carries no reason code,
  being invalid, not-found, or evaluation-unavailable rather than a typed
  conflict, the dialog shall present text specific to that outcome class. An
  evaluation-unavailable outcome shall be presented as retriable and shall keep
  the user's selection; because AC-TASKS-RUNNER-SWITCH-002.18 covers a failed write
  or commit as well as a failed read, a switch that could not be applied arrives in
  this class rather than in a fourth one, and no further outcome class exists for
  the dialog to handle. The dialog shall never render an empty reason or a raw
  code.
- **AC-TASKS-RUNNER-SWITCH-004.4d:** A save that also starts an agent issues a
  session launch, and that launch is a member of the ordered sequence
  AC-TASKS-RUNNER-SWITCH-004.4c governs, not something outside it. It shall be the
  **last** call in the sequence, because it is the only one that materializes the
  task and therefore the only one that can make an earlier call in the same
  sequence irreversible in effect. When the runner switch is rejected, the dialog
  shall issue no launch, for the same reason it issues no field update: a launch
  would materialize the task under the runner the user was just refused permission
  to change. When the switch succeeds, the launch shall carry the switched-to
  profile per AC-TASKS-RUNNER-SWITCH-003.1.
- **AC-TASKS-RUNNER-SWITCH-004.5:** When the user saves without having changed the
  executor profile selection, the dialog shall issue no runner switch. **Changed
  means changed by the user in this editing session.** A value the dialog placed in
  the selector itself, whether by resolving a default, by restoring a remembered
  preference, or by any other seeding it performs on open, is not a user change,
  and the dialog shall not issue a switch on account of it. Value inequality against
  the stored runner is therefore not the test: for a task that stores no executor
  profile at all, a seeded selection necessarily differs from the stored value, and
  treating that difference as a change would make an untouched save silently store
  an explicit runner. AC-TASKS-RUNNER-SWITCH-002.16 makes that consequential, since
  storing where nothing was stored is a real change that pins the task against any
  later change of the resolved default.
- **AC-TASKS-RUNNER-SWITCH-004.5a:** When a task stores no executor profile, the
  dialog shall present the runner the task would resolve to if launched now, and
  shall present it as a resolved default rather than as a stored choice, so a user
  can see what will happen without that display constituting a decision. The task
  shall remain editable in this state per AC-TASKS-RUNNER-SWITCH-001.11: the user
  may choose a runner explicitly, and choosing one that differs from the seeded
  value shall issue a switch.
- **AC-TASKS-RUNNER-SWITCH-004.5b:** When the user's final selection is the value
  the dialog seeded, the dialog shall issue no runner switch, whether the user
  never touched the control or changed it and changed it back. The comparison is
  against the seeded value, not against the history of interactions, so the rule
  has one observable form and does not depend on tracking intermediate states. The
  consequence is deliberate: a user cannot use this surface to pin the resolved
  default as an explicit stored choice, because the dialog cannot distinguish that
  intent from a change reverted. Silently converting a resolved default into a
  durable pin is the worse of the two failures, and it is the one
  AC-TASKS-RUNNER-SWITCH-002.16 would make permanent, so this criterion chooses
  against it. Explicit default-pinning is named in `## Out of scope`.
- **AC-TASKS-RUNNER-SWITCH-004.6:** Desktop and mobile shall use the same gate,
  the same reason text, and the same interaction. This capability introduces no
  new layout and no new mobile interaction pattern.
- **AC-TASKS-RUNNER-SWITCH-004.7:** Every user-visible string this capability
  introduces, including each reason code's presentation, shall be available in
  all shipped locales.
