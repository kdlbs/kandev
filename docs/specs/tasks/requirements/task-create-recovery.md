---
status: active
system: tasks
created: 2026-10-27
owners:
  - kandev
---

# Task creation recovery requirements

## Overview

A lost response can leave a task running while its creation dialog reports an error.
Retrying that submission must recover the same task.
Tasks owns this outcome because it owns creation identity, task allocation, and launch admission.
Desktop and phone recovery belong to this same contract.

This capability covers the standard dialog when it uses Kandev's native REST transport.
It includes Start task, Start task in plan mode, and Create without starting agent.

## Terminology

- **Submission:** One explicit creation action and its submitted values.
- **Receipt:** The browser's retained identity, submitted values, and observed result for that submission.
- **Uncertain result:** The browser cannot prove whether that submission created a task.
- **Incomplete creation:** The matching task exists, but its required creation work is not confirmed complete.
- **Current draft:** The editable form, which can contain changes after the submission.

## Requirements

### REQ-TASKS-CREATE-RECOVERY-001: Recover one intended task creation

**Intent:** A retry recovers one task without allocating another task or launching another agent.

#### Acceptance criteria

- **AC-TASKS-CREATE-RECOVERY-001.1:** When a creation response is lost, an explicit retry shall use the original submission identity. All successful observations shall resolve to the same task while that task retains the identity.
- **AC-TASKS-CREATE-RECOVERY-001.2:** When multiple submission controls activate before the first action settles, the dialog shall admit at most one creation operation. Button, keyboard, plan-mode, create-only, and composer actions shall share this guard.
- **AC-TASKS-CREATE-RECOVERY-001.3:** When the user starts an independent submission after the previous submission resolves, the system shall use a new identity. Identical titles, prompts, profiles, and repositories shall not merge independent submissions.
- **AC-TASKS-CREATE-RECOVERY-001.4:** When a submission remains uncertain, retry shall use its captured creation values and original start intent. Later draft edits shall remain available and shall not silently change an existing task.
- **AC-TASKS-CREATE-RECOVERY-001.5:** When the matching task exists, recovery shall report its observed creation state. An incomplete creation shall remain visible without another allocation, session preparation, agent launch, attachment claim, or prompt submission.
- **AC-TASKS-CREATE-RECOVERY-001.6:** When the first request proves rejection before allocation, the dialog shall retain an editable draft and permit a corrected submission. HTTP status alone shall not prove rejection. After any uncertain attempt, a later rejection or lookup miss shall not authorize a new identity for that submission.
- **AC-TASKS-CREATE-RECOVERY-001.7:** When a submitted native draft survives close, reopen, reconnect, or same-tab browser reload, its receipt shall survive with it. Restoration shall not automatically create a task. Unavailable browser storage shall retain the in-memory receipt and show its persistence limit.
- **AC-TASKS-CREATE-RECOVERY-001.8:** When a late result belongs to another dialog, workspace, or user, it shall not navigate or close the current dialog. It shall preserve the current draft, including newer edits and attachments.
- **AC-TASKS-CREATE-RECOVERY-001.9:** When creation succeeds, the browser shall retain that task identity before callbacks, settings updates, or navigation. A later presentation error shall permit opening that task without another creation request. Completion shall preserve the submitted start, plan-mode, create-only, dependency, and focus preferences.
- **AC-TASKS-CREATE-RECOVERY-001.10:** When a receipt retains attachments, it shall preserve their submitted descriptors without persisting inline file contents or credentials. Retry shall not upload or claim a second copy. A restored submission with unavailable inline contents shall support status recovery without an incomplete payload replay.
- **AC-TASKS-CREATE-RECOVERY-001.11:** When a known task is archived, deleted, or inaccessible, recovery shall not create a replacement or select another task by similarity. Closing the dialog shall not delete a task, release its creation identity, or cancel its agent.
- **AC-TASKS-CREATE-RECOVERY-001.12:** When creation is uncertain, the dialog shall expose Check status and an explicit Retry submission action. Once a task identity is known, Open task shall be available when authorized. Ordinary create actions shall not bypass unresolved recovery.
- **AC-TASKS-CREATE-RECOVERY-001.13:** When users recover on desktop or phones, both surfaces shall expose the same states and actions. Phone actions shall remain visible above the safe area, have targets of at least 44px, and avoid document horizontal scrolling. Status announcements and action labels shall be accessible and localized.
- **AC-TASKS-CREATE-RECOVERY-001.14:** When native creation uses existing validation, attachment uploads, branch consent, queued launches, or workflow selection, recovery shall preserve those contracts. Edit-task, additional-session, Quick Chat, and custom plugin transports shall retain their existing behavior.

## Related contracts

- [External task ID idempotency](external-id-idempotency.md): identity matching and side-effect-free found outcomes.
- [External task ID boundaries](external-id-idempotency-boundaries.md): no caller repair or automatic identity release.
- [Quick Chat opening composer](quick-chat-opening-composer.md): a separate creation endpoint with its own recovery authority.
- [Prompt attachments](prompt-attachments.md): upload ownership and retained descriptors.
- [Create Task Escape dismissal](task-create-escape-dismissal.md): existing dismissal and focus behavior.

## Out of scope

- Diagnosing or correcting the underlying browser or proxy connection failure.
- Cleaning up the three incident tasks or changing their agents.
- Global timeout changes, a database migration, or a new server creation coordinator.
- Extending the plugin host or WebSocket create API with idempotency.
- Automatic repair, deletion, identity release, prompt replay, or creation retries.
- Cross-tab, cross-device, or cross-browser receipt synchronization.
- Server tombstones after physical task deletion or explicit identity release.

## Implementation plan

[Task creation retry protection](../../../plans/task-create-recovery/plan.md).
