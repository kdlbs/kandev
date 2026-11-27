---
status: draft
system: executors
created: 2026-11-27
owners:
  - kandev
---

# Remote credentials on resume

## Overview

Remote sessions need their selected environment credentials after resume as well as on fresh launch. The executors subsystem owns credential provisioning; the secrets subsystem owns visibility and scope.

## Requirements

### REQ-EXECUTORS-RESUME-REMOTE-AUTH-001: Selected credential continuity

**Intent:** Restore selected credentials when rebuilding a remote session's launch environment.

#### Acceptance criteria

- **AC-EXECUTORS-RESUME-REMOTE-AUTH-001.1:** Remote resume shall resolve selected environment authentication methods using the same eligibility and secret-resolution rules as fresh launch. A selected GitHub CLI token shall supply both `GITHUB_TOKEN` and `GH_TOKEN`; selected agent environment credentials shall retain their declared variable names.
- **AC-EXECUTORS-RESUME-REMOTE-AUTH-001.2:** A resumed session using a selected GitHub CLI token shall retain that token for GitHub operations and its runtime environment without requesting a managed GitHub lease. Managed authentication without an explicit token shall retain its existing lease behavior.
- **AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3:** Resume shall retain recorded Kubernetes executor, profile, and workload identity. Local and worktree sessions shall retain their existing credential behavior. Secret reads shall retain the caller's visibility and scope; unreadable remote authentication selections shall retain fresh-launch handling.

## Out of scope

New authentication methods, secret-sharing permissions, credential caches, provider token validation, and changes to missing-secret policy.

## Related contracts

- [Kubernetes failure recovery](kubernetes-failure-recovery.md)
- [Repository and workspace secrets](../../workspaces/requirements/repository-secrets.md)
