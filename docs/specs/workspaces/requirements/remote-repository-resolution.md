---
status: active
system: workspaces
created: 2026-10-02
owners:
  - kandev
---

# Remote repository resolution requirements

## Overview

Selecting a remote repository must remain usable after a previously registered
local checkout is deleted. The workspace system owns repository registration
and selection; the task system consumes the selected repository for launch.

## Requirements

### REQ-WORKSPACES-REMOTE-RESOLUTION-001: Select an available repository source

**Intent:** Start new work from a remote repository without restoring an unrelated
deleted local checkout or changing its saved identity.

#### Acceptance criteria

- **AC-WORKSPACES-REMOTE-RESOLUTION-001.1:** When a user selects a remote
  repository whose matching saved local checkout directory has been deleted,
  Kandev shall select an available matching repository or prepare a managed
  clone. The new task shall start when normal remote access and launch
  prerequisites succeed.
- **AC-WORKSPACES-REMOTE-RESOLUTION-001.2:** If at least one eligible matching
  repository exists in the selected workspace, a deleted local candidate shall
  not prevent its selection. Repeated and concurrent selections shall reuse
  that eligible registration rather than create additional managed registrations.
- **AC-WORKSPACES-REMOTE-RESOLUTION-001.3:** Remote fallback shall preserve the
  original local registration, its saved path, settings, and existing task
  associations. It shall not recreate, overwrite, or delete the original
  checkout. Explicit local-path or repository-ID selection shall retain its
  existing local ownership behavior.
- **AC-WORKSPACES-REMOTE-RESOLUTION-001.4:** A matching local checkout that
  remains available and passes normal repository validation shall retain its
  current reuse behavior. Permission failures, changed canonical identity,
  invalid Git metadata, and other inspection errors shall not be treated as
  proof that the directory was deleted.
- **AC-WORKSPACES-REMOTE-RESOLUTION-001.5:** Candidate selection shall preserve
  workspace and provider identity isolation, including host and scoped provider
  identities. A foreign or ambiguously identified repository shall not satisfy
  the request.
- **AC-WORKSPACES-REMOTE-RESOLUTION-001.6:** Managed fallback shall use the
  requested repository and branch under the existing credential policy.
  Authentication, clone, and cancellation failures shall propagate through the
  current failure flow without starting an agent in the deleted local path.

## Out of scope

- Restoring already materialized task worktrees or their lost Git objects.
- Repairing explicit local selections, inaccessible mounts, or invalid checkouts.
- Changing clone credentials, remote checkout options, or picker presentation.
- Copying machine-specific settings or secrets from the skipped local registration.

## Related contracts

- [Local repository requirements](local-repositories.md)
- [Managed clone relocation](../../tasks/requirements/managed-clone-relocation.md)
- [System design](../system-design/remote-repository-resolution.md)
