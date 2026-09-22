---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Same-workspace refusal Requirements

## Overview

This action exists only for a target workspace different from the caller's
own. The same-workspace case is refused early and named clearly, and the
existing same-workspace creation path and route are otherwise untouched. The
delivery task itself must also be observably an ordinary kanban task, not an
Office task, in the target workspace it lands in.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001: Refusal, and the delivery task's own workspace identity

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.1:** When
  `target_workspace_id` equals the caller's own workspace, as derived from
  the run token, the call shall be refused with HTTP 400 and a message
  naming the existing same-workspace task-creation command as the path for
  that case. The comparison shall use the run token's workspace, never a
  payload-supplied value, and shall run before the permission check, so a
  caller that targets itself is told what it actually did wrong rather than
  told it lacks a permission.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.2:** The existing
  same-workspace task-creation route and action shall be unchanged: no new
  field, capability, or workspace input on that path.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.3:** The delivery task
  shall be observably an ordinary kanban task: created with no origin
  supplied by this action, no project association, and in the caller-supplied
  target workflow. It shall therefore receive no office task identifier, and
  the target workspace's task-identifier sequence shall not be incremented by
  this call. This shall be true in a way that is directly testable — the
  stored origin, the empty identifier, and the unchanged task sequence — and
  not solely inferred from any single read-time projection, since that
  projection alone would also be true of an office-triggering origin this
  requirement forbids.
