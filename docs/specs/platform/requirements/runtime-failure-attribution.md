---
status: active
system: platform
created: 2026-09-24
owners:
  - kandev
---

# Runtime failure attribution requirements

## Overview

Several operational warnings identify a failed operation but not the boundary
that failed. Platform owns the cross-cutting diagnostic contract. The owning
systems retain their health, webhook, repository, and recovery behavior.

## Requirements

### REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001: Bounded failure attribution

**Intent:** An operator can distinguish a host-side failure, a plugin response,
an identity rejection, and expected non-recovery without exposing request data.

#### Acceptance criteria

- **AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1:** When a required persistence
  probe fails, one bounded diagnostic shall identify the failing probe stage,
  elapsed time, error class, and writer/reader connection-pool pressure. The
  existing health transition and recovery behavior shall remain unchanged.
- **AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.2:** When a plugin webhook
  returns a server error, a diagnostic shall distinguish host lifecycle failure,
  host RPC failure, and a plugin-supplied response. It shall identify the plugin
  and status without logging webhook keys, request contents, or credentials.
- **AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.3:** When a live pull request is
  rejected by the task-repository identity guard, a bounded diagnostic shall
  identify the comparison that failed. The same mismatch shall continue to
  block launch or follow the existing safe fallback path.
- **AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4:** When startup session
  recovery finishes below its inventory count, diagnostics shall distinguish
  retracked records from records not retracked and classify known reasons.
  Unknown liveness shall not be treated as absence or trigger cleanup.

## Out of scope

- Changing probe timeouts or readiness policy before the database stall is attributed.
- Changing plugin-provided webhook responses or external plugin manifests.
- Relaxing the pull-request identity guard or suppressing the startup progress warning.
