---
status: draft
system: platform
created: 2026-10-05
owners:
  - kandev
---

# Workspace Git capture recovery requirements

## Overview

Platform owns shared Git observations and their delivery to consumers.
A brief worktree change must not become a permanent capture failure.
Tasks retains ownership of saved session baselines.
The existing [Git status contract](workspace-git-status.md) governs snapshot validity and publication.

## Requirements

### REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001: Bounded capture recovery

**Intent:** Recover from brief capture races without accepting mixed Git evidence.

#### Acceptance criteria

- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.1:** When evidence changes during basic capture, the shared observation shall permit one corrective capture within its existing deadline.
  A stable corrective result shall reach waiting consumers and normal subscribers.
- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2:** Continuous changes shall cause a bounded unavailable result.
  Neither attempt shall publish incomplete membership or combine evidence from different captures.
- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.3:** Overlapping consumers shall share corrective work within the existing repository and admission scope.
  Cancellation of one consumer shall not cancel another consumer's observation.
- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4:** Permission failures, missing repositories, expired deadlines, and tracker shutdown shall not trigger a corrective capture.
  Existing failure classification and cleanup shall remain effective.
- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5:** When a stable enriched result follows a brief race, launch baseline capture shall retain the observed comparison baseline.
  Unavailable enrichment shall not authorize a saved pending value or an unrelated HEAD baseline.
- **AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.6:** For compatibility, a successful legacy status payload with all quality fields omitted may establish the baseline. If any quality field is present, the payload must describe a successful, complete, ready status.

## Out of scope

- Changing Git refs, index content, working files, credentials, or transport.
- Increasing observation deadlines or removing content validation.
- Replacing enriched baseline capture with a new metadata-only endpoint.
- Changing Changes layouts, status DTOs, or frontend retry schedules.

## Related documents

- [System design](../system-design/workspace-git-capture-recovery.md)
- [Implementation plan](../../../plans/runtime-log-reliability/plan.md)
