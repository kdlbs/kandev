---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
---

# Recovery workspace availability and operation causes

## Boundary and requirement mapping

This design extends
[session recovery failures](session-recovery-failures.md). It maps to
`AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3`, `006.2`, `006.12`, `006.18`,
and `006.28`. Tasks owns whether a retry is admitted. See
[initial-target retry before route commit](../../tasks/system-design/initial-target-first-launch-retry.md).

## Workspace restorability

Session failure metadata (the `recovery_actions` producer in
`event_handlers_agent.go`) includes `workspace_restorable`. The value comes
from the session's task environment and uses the workspace-admission rule:
the environment exists, its status is empty, `ready`, or `stopped`, and it has
a workspace path. A read error leaves the key out. An absent key means unknown
and keeps the current restore choice.

When `workspace_restorable` is `false`, the card omits
**Restore read-only workspace**. It shows a localized prerequisite line saying
that no workspace was created. Other eligible actions are unchanged.

| Cause or capability | Primary | Secondary/prerequisite |
| --- | --- | --- |
| No checkout was created | Existing eligible fresh start or retry | Omit workspace restore and explain that no workspace exists |

## Operation-specific failure titles

A failed manual recovery records its operation. `resume` and
`resume_new_branch` record `resume`. `fresh_start` and `runtime_retry` record
`fresh_start`. Workspace restoration records `restore_workspace`.

The failure title names the recorded operation, for example
**Failed to start fresh session**. The sanitized backend cause stays in the
initially collapsed details. Guard and branch-loss copy keep their precedence.
Desktop and phone render the same content.

## Implementation plans

- [Initial-session first-launch retry](../../../plans/initial-session-first-launch-retry/plan.md)
