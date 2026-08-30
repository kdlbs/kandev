---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-001
---

# Local Workspace Repository Validation

## Boundary

`internal/task/service` canonicalizes an explicit repository path before it is
saved or used. Automatic discovery roots do not constrain explicit selection.

## Metadata validation

Standalone `.git` directories remain valid when they do not redirect their
common directory. A regular-file `.git` pointer is accepted through one of two
independent reciprocal validators: linked worktrees require `gitdir`,
`commondir`, and placement under `<common>/worktrees`; initialized submodules
require a non-empty `[core] worktree` value in module metadata. Relative values
resolve from the canonical metadata directory and must canonically equal the
selected repository. A `commondir` file excludes the submodule validator. Git
include sections and `extensions.worktreeConfig` are rejected for submodule
metadata because the validator does not evaluate alternate configuration
sources.

When neither validator succeeds, their errors are joined to retain diagnostics.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-WORKSPACES-LOCAL-REPOSITORIES-001 | Boundary and Metadata validation |

## Related ADRs

- [Explicit submodule repository trust](../../../decisions/2026-08-28-explicit-submodule-repository-trust.md)
